// tspldriver - unified binary: CLI / CUPS filter / CUPS backend
// Author: Celso Inacio <celso (at) enssure (dot) com>
// SPDX-License-Identifier: MIT
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/go-fitz"
)

// ----------------- Defaults (overridable via CLI or CUPS options) -------------
var (
	DPI        = 200
	LABEL_W_MM = 100.0
	LABEL_H_MM = 150.0
	MM_TO_IN   = 0.0393701
	MARGIN_MM  = 2.0
	GAP_MM     = 2.0
	DELAY_MS   = 200
)

var (
	PX_W      int
	PX_H      int
	MARGIN_PX int
)

func recalcPixels() {
	PX_W = int(math.Round(LABEL_W_MM * MM_TO_IN * float64(DPI)))
	PX_H = int(math.Round(LABEL_H_MM * MM_TO_IN * float64(DPI)))
	MARGIN_PX = int(math.Round(MARGIN_MM * MM_TO_IN * float64(DPI)))
}

// ----------------- Logging helpers -------------------------------------------
func logInfo(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "I: "+format+"\n", a...)
}
func logErr(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "E: "+format+"\n", a...)
}

// ----------------- PDF -> PNG (pages) ---------------------------------------
func pdfToPngPages(pdfPath string, tmpDir string) ([]string, error) {
	logInfo("Converting PDF to PNG at %ddpi ...", DPI)

	doc, err := fitz.New(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer doc.Close()

	var pages []string
	for i := 0; i < doc.NumPage(); i++ {
		img, err := doc.ImageDPI(i, float64(DPI))
		if err != nil {
			return nil, fmt.Errorf("render page %d: %w", i+1, err)
		}
		out := filepath.Join(tmpDir, fmt.Sprintf("page-%d.png", i+1))
		f, err := os.Create(out)
		if err != nil {
			return nil, fmt.Errorf("create png: %w", err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return nil, fmt.Errorf("encode png: %w", err)
		}
		f.Close()
		pages = append(pages, out)
	}

	sort.Strings(pages)
	logInfo("PDF -> PNG produced %d pages", len(pages))
	return pages, nil
}

// // ------------------------------------------------------------
// // Crop an A4 page PNG into labels of size PX_W x PX_H (pixels)
// // This function handles partial labels at the bottom by centering
// // and filling with white background instead of failing.
// // Returns a slice of file paths (saved PNGs) for debugging and further processing.
// // ------------------------------------------------------------
func cropToLabels(pagePng string, outDir string) ([]string, error) {
	logInfo("Cropping page %s into labels (px %dx%d)...", pagePng, PX_W, PX_H)
	img, err := imaging.Open(pagePng)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	pageW := b.Dx()
	pageH := b.Dy()

	logInfo("Page dimensions: %dx%d pixels", pageW, pageH)
	logInfo("Label size: %dx%d pixels", PX_W, PX_H)
	logInfo("Margin: %dmm = %dpx", int(MARGIN_MM), MARGIN_PX)

	rows := int(math.Ceil(float64(pageH) / float64(PX_H)))

	logInfo("Calculated rows: %d (pageH=%d / PX_H=%d = %d)", rows, pageH, PX_H, pageH/PX_H)
	logInfo("Remainder: %d pixels", pageH%PX_H)

	var labels []string
	for r := 0; r < rows; r++ {
		left := 0
		top := r * PX_H

		logInfo("Cropping label %d at position: left=%d top=%d (size: %dx%d)",
			r+1, left, top, PX_W, PX_H)

		rect := image.Rect(left, top, left+PX_W, top+PX_H)
		cropped := imaging.Crop(img, rect)
		// ensure exact size
		cropped = imaging.Resize(cropped, PX_W, PX_H, imaging.Lanczos)

		innerW := PX_W - (2 * MARGIN_PX)
		innerH := PX_H - (2 * MARGIN_PX)

		cropped = imaging.Fit(cropped, innerW, innerH, imaging.Lanczos)

		canvas := imaging.New(PX_W, PX_H, color.NRGBA{255, 255, 255, 255})

		canvas = imaging.PasteCenter(canvas, cropped)

		var buf bytes.Buffer
		if err := png.Encode(&buf, canvas); err != nil {
			return nil, err
		}

		buffer := buf.Bytes()

		outPath := filepath.Join(outDir, fmt.Sprintf("%02d_label%02d.png", time.Now().UnixMilli(), r+1))
		_ = ioutil.WriteFile(outPath, buffer, 0o644)

		labels = append(labels, outPath)
	}
	logInfo("Cropped into %d labels", len(labels))
	return labels, nil
}

// ----------------- PNG -> TSPL (bitmap) ------------------------------------
func pngToTsplFromBuffer(pngBuf []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngBuf))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	gray := imaging.Grayscale(img)
	b := gray.Bounds()
	w := b.Dx()
	h := b.Dy()

	// ensure expected size
	if w != PX_W || h != PX_H {
		gray = imaging.Resize(gray, PX_W, PX_H, imaging.Lanczos)
		b = gray.Bounds()
		w = b.Dx()
		h = b.Dy()
	}

	// pad width to multiple of 8 (TSPL expects byte-aligned width)
	paddedW := (w + 7) &^ 7
	if paddedW != w {
		logInfo("Padding width from %d -> %d (TSPL requirement)", w, paddedW)
		padded := imaging.New(paddedW, h, color.NRGBA{255, 255, 255, 255})
		padded = imaging.Paste(padded, gray, image.Pt(0, 0))
		gray = padded
		w = paddedW
	}

	bytesPerRow := w / 8
	bitmap := make([]byte, bytesPerRow*h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.GrayModel.Convert(gray.At(b.Min.X+x, b.Min.Y+y)).(color.Gray)
			var bit byte
			if c.Y < 128 {
				bit = 1 // dark pixel
			} else {
				bit = 0 // bright pixel
			}
			// invert as in your Node.js: bit = 1 - bit
			bit = 1 - bit

			byteIndex := y*bytesPerRow + (x >> 3)
			bitmap[byteIndex] |= bit << (7 - (x & 7))
		}
	}

	header := fmt.Sprintf("SIZE %.0f mm,%.0f mm\nGAP %.0f mm,0 mm\nCLS\nBITMAP 0,0,%d,%d,1,", LABEL_W_MM, LABEL_H_MM, GAP_MM, bytesPerRow, h)
	out := new(bytes.Buffer)
	out.WriteString(header)
	out.Write(bitmap)
	out.WriteString("\nPRINT 1\n")
	return out.Bytes(), nil
}

// ----------------- Write TSPL to device -------------------------------------
func writeToPrinter(tspl []byte, dev string) error {
	logInfo("Writing %d bytes to printer %s", len(tspl), dev)

	// If device looks like "tspl:/dev/usb/lp5" or "file:/dev/usb/lp5", extract path
	if strings.Contains(dev, ":") {
		// split scheme
		parts := strings.SplitN(dev, ":", 2)
		if len(parts) == 2 {
			// allow tspl:/dev/usb/lp5 or file:///dev/usb/lp5
			path := parts[1]
			// strip leading slashes for file:///
			path = strings.TrimPrefix(path, "//")
			dev = path
		}
	}

	info, err := os.Stat(dev)
	if err != nil {
		return fmt.Errorf("printer device not found: %w", err)
	}
	logInfo("Device exists: %s (mode=%v)", dev, info.Mode())

	f, err := os.OpenFile(dev, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer f.Close()

	chunk := 4096
	w := 0
	for w < len(tspl) {
		end := w + chunk
		if end > len(tspl) {
			end = len(tspl)
		}
		n, err := f.Write(tspl[w:end])
		if err != nil {
			return fmt.Errorf("write error at %d: %w", w, err)
		}
		w += n
		time.Sleep(20 * time.Millisecond)
	}
	if err := f.Sync(); err != nil {
		logErr("sync failed: %v", err)
	}
	// close happens by defer
	// give printer a little time to process and advance
	time.Sleep(300 * time.Millisecond)
	logInfo("Wrote %d bytes", w)
	return nil
}

// ----------------- CUPS options parser (options string like "PageSize=100x150mm Dpi=203") ----------
func parseCupsOptions(opts string) {
	parts := strings.Fields(opts)
	for _, p := range parts {
		if strings.Contains(p, "=") {
			k, v, _ := strings.Cut(p, "=")
			k = strings.ToLower(k)
			switch k {
			case "pagesize":
				v = strings.ToLower(v)
				v = strings.TrimSuffix(v, "mm")
				if strings.Contains(v, "x") {
					w, h := parseTwoFloats(v)
					LABEL_W_MM = w
					LABEL_H_MM = h
				}
			case "dpi":
				DPI = parseInt(v)
			case "margin":
				MARGIN_MM = parseFloat(v)
			case "gap":
				GAP_MM = parseFloat(v)
			case "delay":
				DELAY_MS = parseInt(v)
			}
		}
	}
	recalcPixels()
}

func parseTwoFloats(s string) (float64, float64) {
	parts := strings.Split(s, "x")
	if len(parts) != 2 {
		return LABEL_W_MM, LABEL_H_MM
	}
	return parseFloat(parts[0]), parseFloat(parts[1])
}
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ----------------- Utility ensure dir ---------------------------------------
func ensureDir(p string) {
	_ = os.MkdirAll(p, 0o755)
}

// ----------------- MODE: FILTER (CUPS filter) --------------------------------
// CUPS filter invocation: filter job-id user title copies options [filename]
// argv[0] = filter path
// argv[1] = job-id
// argv[2] = user
// argv[3] = title
// argv[4] = copies
// argv[5] = options
// argv[6] = filename (optional, if missing read from stdin)
func modeFilter(argv []string) error {
	logInfo("Filter mode started with %d args", len(argv))
	for i, arg := range argv {
		logInfo("  argv[%d] = %s", i, arg)
	}

	// Parse CUPS filter arguments
	var pdfPath string
	var options string

	if len(argv) >= 6 {
		options = argv[5]
		logInfo("CUPS options: %s", options)
	}

	if len(argv) >= 7 {
		// File provided as argument
		pdfPath = argv[6]
		logInfo("Input file: %s", pdfPath)
	} else {
		// Read from stdin and save to temp file
		logInfo("Reading PDF from stdin...")
		tmpDir := "/tmp/tspl_filter"
		ensureDir(tmpDir)
		pdfPath = filepath.Join(tmpDir, fmt.Sprintf("input-%d.pdf", time.Now().Unix()))

		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		logInfo("Read %d bytes from stdin", len(data))

		if err := ioutil.WriteFile(pdfPath, data, 0644); err != nil {
			return fmt.Errorf("write temp pdf: %w", err)
		}
		logInfo("Saved to temp file: %s", pdfPath)
		defer os.Remove(pdfPath)
	}

	// Verify file exists
	if _, err := os.Stat(pdfPath); err != nil {
		return fmt.Errorf("pdf file not found: %s (%w)", pdfPath, err)
	}

	// tmp/out
	tmpDir := "/tmp/tspl_pages"
	outDir := "/tmp/tspl_labels"
	ensureDir(tmpDir)
	ensureDir(outDir)

	// parse options if any
	if options != "" {
		parseCupsOptions(options)
	}

	recalcPixels()

	// Render PDF pages
	pages, err := pdfToPngPages(pdfPath, tmpDir)
	if err != nil {
		return fmt.Errorf("pdfToPngPages: %w", err)
	}
	logInfo("Filter: pages=%d", len(pages))

	// For each page -> crop -> labels -> tspl -> write to stdout sequentially
	for i, pg := range pages {
		labels, err := cropToLabels(pg, outDir)
		if err != nil {
			logErr("cropToLabels (%s): %v", pg, err)
			continue
		}
		logInfo("Filter: page %d -> %d labels", i+1, len(labels))
		for j, lbl := range labels {
			raw, err := ioutil.ReadFile(lbl)
			if err != nil {
				logErr("read label (%s): %v", lbl, err)
				continue
			}
			tspl, err := pngToTsplFromBuffer(raw)
			if err != nil {
				logErr("pngToTspl: %v", err)
				continue
			}
			// write TSPL to stdout (CUPS filter expects output on stdout)
			if _, err := os.Stdout.Write(tspl); err != nil {
				return fmt.Errorf("stdout write: %w", err)
			}
			// small delay between labels
			time.Sleep(time.Duration(DELAY_MS) * time.Millisecond)
			logInfo("Filter: wrote page %d label %d", i+1, j+1)
		}
	}

	return nil
}

// ----------------- MODE: BACKEND (CUPS backend) --------------------------------
// Backend is invoked by CUPS to send data to the device. When invoked with "list" should list devices.
// When invoked as job, last argument typically is filename containing data (TSPL) or "-" to read stdin.
func modeBackend(argv []string) error {
	// If called as "list" -> list available device URIs that this backend can handle.
	// We'll look for /dev/usb/lp* or allow env override TSPL_DEVICES
	if len(argv) > 1 && argv[len(argv)-1] == "list" {
		// list available USB printers
		matches, _ := filepath.Glob("/dev/usb/lp*")
		if len(matches) == 0 {
			fmt.Println("direct tspl:/dev/usb/lp5 \"TSPL USB Printer\"")
			return nil
		}
		for _, m := range matches {
			fmt.Printf("direct tspl:%s \"TSPL USB Printer\"\n", m)
		}
		return nil
	}

	// job invocation - CUPS may call backend with args:
	// argv[0]=backend, argv[1]=job-id, argv[2]=user, argv[3]=title, argv[4]=copies, argv[5]=options, argv[6]=filename
	// But sometimes it is: backend uri jobid user title copies options filename
	// We'll parse from end: last arg is filename or "-" and device may be passed via job URI (argv[1] or argv[0]).
	if len(argv) < 2 {
		return fmt.Errorf("backend: insufficient args")
	}

	// try to find filename at the end
	filename := ""
	if len(argv) > 1 {
		filename = argv[len(argv)-1]
	}
	// try to detect device from env or argv
	// If argv[0] contains ":" we might have scheme. We'll accept TSPL_DEVICE env override.
	dev := os.Getenv("TSPL_DEVICE")
	if dev == "" {
		if len(argv) > 0 && strings.Contains(argv[0], ":") {
			parts := strings.SplitN(argv[0], ":", 2)
			dev = parts[1]
		}
	}
	if dev == "" {
		dev = "/dev/usb/lp5"
	}

	var tspl []byte
	var err error
	if filename == "-" || filename == "" {
		tspl, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		tspl, err = ioutil.ReadFile(filename)
		if err != nil {
			logInfo("Backend: file read failed (%v), trying to convert as PDF", err)
			tmpDir := "/tmp/tspl_pages"
			outDir := "/tmp/tspl_labels"
			ensureDir(tmpDir)
			ensureDir(outDir)
			recalcPixels()
			pages, perr := pdfToPngPages(filename, tmpDir)
			if perr != nil {
				return fmt.Errorf("read file and convert failed: %w", perr)
			}
			var combined bytes.Buffer
			for _, pg := range pages {
				labels, cerr := cropToLabels(pg, outDir)
				if cerr != nil {
					logErr("cropToLabels: %v", cerr)
					continue
				}
				for _, lbl := range labels {
					raw, rerr := ioutil.ReadFile(lbl)
					if rerr != nil {
						logErr("read label: %v", rerr)
						continue
					}
					ts, terr := pngToTsplFromBuffer(raw)
					if terr != nil {
						logErr("pngToTspl: %v", terr)
						continue
					}
					combined.Write(ts)
					time.Sleep(time.Duration(DELAY_MS) * time.Millisecond)
				}
			}
			tspl = combined.Bytes()
		}
	}

	if len(tspl) == 0 {
		return fmt.Errorf("no data to write")
	}

	if strings.HasPrefix(dev, "tspl:") || strings.HasPrefix(dev, "file:") {
		parts := strings.SplitN(dev, ":", 2)
		dev = strings.TrimPrefix(parts[1], "//")
	}
	logInfo("Backend writing to device %s (bytes=%d)", dev, len(tspl))

	if err := writeToPrinter(tspl, dev); err != nil {
		return fmt.Errorf("writeToPrinter: %w", err)
	}
	return nil
}

func modeCLI(pdfPath string, printer string, options string) error {
	if options != "" {
		parseCupsOptions(options)
	}
	recalcPixels()

	tmpDir := "./tmp_tspl"
	outDir := "./out_tspl"
	ensureDir(tmpDir)
	ensureDir(outDir)

	pages, err := pdfToPngPages(pdfPath, tmpDir)
	if err != nil {
		return fmt.Errorf("pdfToPngPages: %w", err)
	}

	total := 0
	for i, pg := range pages {
		labels, err := cropToLabels(pg, outDir)
		if err != nil {
			logErr("cropToLabels: %v", err)
			continue
		}
		for j, lbl := range labels {
			raw, err := ioutil.ReadFile(lbl)
			if err != nil {
				logErr("read label: %v", err)
				continue
			}
			tspl, err := pngToTsplFromBuffer(raw)
			if err != nil {
				logErr("pngToTspl: %v", err)
				continue
			}
			if err := writeToPrinter(tspl, printer); err != nil {
				return fmt.Errorf("writeToPrinter: %w", err)
			}
			total++
			time.Sleep(time.Duration(DELAY_MS) * time.Millisecond)
			logInfo("Printed page %d label %d", i+1, j+1)
		}
	}

	logInfo("CLI done: printed %d labels", total)
	return nil
}

func detectMode() string {
	// flags override
	modeFlag := flag.Lookup("mode")
	if modeFlag != nil {
	}

	name := filepath.Base(os.Args[0])
	name = strings.ToLower(name)
	if strings.Contains(name, "backend") {
		return "backend"
	}
	if strings.Contains(name, "filter") {
		return "filter"
	}
	return "cli"
}

// ----------------- main ------------------------------------------------------
func main() {
	mode := flag.String("mode", "", "mode: cli|filter|backend (auto-detected by executable name if empty)")
	dpi := flag.Int("dpi", 0, "override dpi")
	width := flag.Float64("width", 0, "label width mm override")
	height := flag.Float64("height", 0, "label height mm override")
	margin := flag.Float64("margin", 0, "margin mm override")
	gap := flag.Float64("gap", 0, "gap mm override")
	delay := flag.Int("delay", 0, "delay ms override")
	flag.Parse()

	// detect mode if not set
	finalMode := *mode
	if finalMode == "" {
		finalMode = detectMode()
	}

	args := flag.Args()

	// apply CLI overrides
	if *dpi > 0 {
		DPI = *dpi
	}
	if *width > 0 {
		LABEL_W_MM = *width
	}
	if *height > 0 {
		LABEL_H_MM = *height
	}
	if *margin > 0 {
		MARGIN_MM = *margin
	}
	if *gap > 0 {
		GAP_MM = *gap
	}
	if *delay > 0 {
		DELAY_MS = *delay
	}

	recalcPixels()

	// route modes
	switch finalMode {
	case "filter":
		// CUPS filter mode: receives job-id user title copies options [filename]
		if err := modeFilter(os.Args); err != nil {
			logErr("filter error: %v", err)
			os.Exit(2)
		}
	case "backend":
		if err := modeBackend(os.Args); err != nil {
			logErr("backend error: %v", err)
			os.Exit(3)
		}
	default: // cli
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage:\n CLI: tspldriver [--dpi=203 --width=100 --height=150] <pdf> <printer> [options-string]\n")
			os.Exit(1)
		}
		pdfPath := args[0]
		printer := "/dev/usb/lp5"
		options := ""
		if len(args) >= 2 {
			printer = args[1]
		}
		if len(args) >= 3 {
			options = args[2]
		}
		if err := modeCLI(pdfPath, printer, options); err != nil {
			logErr("cli error: %v", err)
			os.Exit(4)
		}
	}
}
