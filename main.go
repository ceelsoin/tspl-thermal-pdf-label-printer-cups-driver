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
	DPI                  = 200
	LABEL_W_MM           = 100.0
	LABEL_H_MM           = 150.0
	MM_TO_IN             = 0.0393701
	MARGIN_MM            = 2.0
	GAP_MM               = 2.0
	DELAY_MS             = 200
	SAFE_MARGIN_RIGHT_MM = 4.0
	SAFE_MARGIN_RIGHT_PX = int(math.Round(SAFE_MARGIN_RIGHT_MM * MM_TO_IN * float64(DPI)))
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

func isImageBlank(img image.Image, threshold uint8) bool {
	bounds := img.Bounds()
	whitePixels := 0
	totalPixels := (bounds.Dx() * bounds.Dy())

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// Normalizar para 0-255
			r = r >> 8
			g = g >> 8
			b = b >> 8
			a = a >> 8

			// Considerar pixel como branco se RGB > threshold e A = 255
			if r > uint32(threshold) && g > uint32(threshold) && b > uint32(threshold) && a == 255 {
				whitePixels++
			}
		}
	}

	// Se mais de 95% da imagem é branca, considera em branco
	return float64(whitePixels)/float64(totalPixels) > 0.95
}

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

	// Grid 2x2 (até 4 labels por página)
	rows := 2
	cols := 2

	// Calcular quantas labels cabem realmente (limitado a 2 linhas, 2 colunas)
	maxRows := int(math.Ceil(float64(pageH) / float64(PX_H)))
	if maxRows < rows {
		rows = maxRows
	}

	maxCols := int(math.Ceil(float64(pageW) / float64(PX_W)))
	if maxCols < cols {
		cols = maxCols
	}

	logInfo("Grid: %d rows x %d cols (max based on page: %dx%d)", rows, cols, maxRows, maxCols)

	var labels []string
	labelIndex := 1

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			left := c * PX_W
			top := r * PX_H

			// Aplicar margem segura para coluna direita (c=1)
			if c == 1 {
				left += SAFE_MARGIN_RIGHT_PX + 25
			} else if c == 0 {
				left += SAFE_MARGIN_RIGHT_PX - 25
			}

			// Validar se está dentro dos limites
			if left >= pageW || top >= pageH {
				logInfo("Label position %d skipped: out of bounds (left=%d top=%d, page=%dx%d)", labelIndex, left, top, pageW, pageH)
				labelIndex++
				continue
			}

			// Ajustar rect para não ultrapassar limites
			right := left + PX_W
			bottom := top + PX_H

			if right > pageW {
				right = pageW
			}
			if bottom > pageH {
				bottom = pageH
			}

			logInfo("Cropping label %d at position: left=%d top=%d right=%d bottom=%d (size: %dx%d)",
				labelIndex, left, top, right, bottom, right-left, bottom-top)

			rect := image.Rect(left, top, right, bottom)
			cropped := imaging.Crop(img, rect)

			// Verificar se está em branco antes de processar
			if isImageBlank(cropped, 240) {
				logInfo("Label %d is blank, skipping", labelIndex)
				labelIndex++
				continue
			}

			// Redimensionar para tamanho exato (PX_W x PX_H)
			cropped = imaging.Resize(cropped, PX_W, PX_H, imaging.Lanczos)

			// Aplicar margens
			innerW := PX_W - (2 * MARGIN_PX)
			innerH := PX_H - (2 * MARGIN_PX)

			if innerW > 0 && innerH > 0 {
				cropped = imaging.Fit(cropped, innerW, innerH, imaging.Lanczos)
			}

			// Canvas branco com label centralizada
			canvas := imaging.New(PX_W, PX_H, color.NRGBA{255, 255, 255, 255})
			canvas = imaging.PasteCenter(canvas, cropped)

			var buf bytes.Buffer
			if err := png.Encode(&buf, canvas); err != nil {
				return nil, err
			}

			buffer := buf.Bytes()
			outPath := filepath.Join(outDir, fmt.Sprintf("%02d_label%02d.png", time.Now().UnixMilli(), labelIndex))

			if err := ioutil.WriteFile(outPath, buffer, 0o644); err != nil {
				logInfo("Error writing file %s: %v", outPath, err)
				labelIndex++
				continue
			}

			logInfo("Saved label %d: %s", labelIndex, outPath)
			labels = append(labels, outPath)
			labelIndex++
		}
	}

	logInfo("Cropped into %d non-blank labels from page", len(labels))
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

	if len(argv) >= 7 && argv[6] != "-" {
		// File provided as argument (not stdin marker)
		pdfPath = argv[6]
		logInfo("Input file: %s", pdfPath)

		// Verify file exists
		if _, err := os.Stat(pdfPath); err != nil {
			return fmt.Errorf("pdf file not found: %s (%w)", pdfPath, err)
		}
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
// Backend is invoked by CUPS to send data to the device.
// CUPS calls backend with: device-uri job-id user title copies options [file]
// If file is not provided, data comes from stdin (piped from filter).
func modeBackend(argv []string) error {
	logInfo("Backend mode started with %d args", len(argv))
	for i, arg := range argv {
		logInfo("  backend argv[%d] = %s", i, arg)
	}

	// If called as "list" -> list available device URIs
	if len(argv) == 1 || (len(argv) > 1 && argv[len(argv)-1] == "list") {
		matches, _ := filepath.Glob("/dev/usb/lp*")
		if len(matches) == 0 {
			fmt.Println("direct tspl:/dev/usb/lp5 \"TSPL USB Printer\" \"TSPL Thermal Label Printer\"")
			return nil
		}
		for _, m := range matches {
			fmt.Printf("direct tspl:%s \"TSPL USB Printer\" \"TSPL Thermal Label Printer\"\n", m)
		}
		return nil
	}

	// CUPS backend invocation:
	// argv[0] = device-uri (e.g., tspl:/dev/usb/lp5)
	// argv[1] = job-id
	// argv[2] = user
	// argv[3] = title
	// argv[4] = copies
	// argv[5] = options
	// argv[6] = file (optional - if missing, read from stdin)
	if len(argv) < 6 {
		return fmt.Errorf("backend: insufficient args (need at least 6, got %d)", len(argv))
	}

	// Extract device from URI (argv[0])
	dev := os.Getenv("TSPL_DEVICE")
	if dev == "" {
		deviceURI := argv[0]
		if strings.HasPrefix(deviceURI, "tspl:") {
			dev = strings.TrimPrefix(deviceURI, "tspl:")
			dev = strings.TrimPrefix(dev, "//")
		} else if strings.HasPrefix(deviceURI, "file:") {
			dev = strings.TrimPrefix(deviceURI, "file:")
			dev = strings.TrimPrefix(dev, "//")
		} else {
			dev = deviceURI
		}
	}
	if dev == "" {
		dev = "/dev/usb/lp5"
	}

	// Determine if we have a file argument or should read from stdin
	// If argv[6] exists and is not "-", it's a file path
	var tspl []byte
	var err error

	if len(argv) >= 7 && argv[6] != "" && argv[6] != "-" {
		filename := argv[6]
		logInfo("Backend: reading from file %s", filename)
		tspl, err = ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("backend: failed to read file %s: %w", filename, err)
		}
		logInfo("Backend: read %d bytes from file", len(tspl))
	} else {
		// Read from stdin (data piped from filter)
		logInfo("Backend: reading TSPL from stdin...")
		tspl, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		logInfo("Backend: read %d bytes from stdin", len(tspl))
	}

	if len(tspl) == 0 {
		return fmt.Errorf("no data to write (got 0 bytes)")
	}

	logInfo("Backend: writing to device %s (bytes=%d)", dev, len(tspl))

	if err := writeToPrinter(tspl, dev); err != nil {
		return fmt.Errorf("writeToPrinter: %w", err)
	}

	logInfo("Backend: successfully wrote %d bytes to %s", len(tspl), dev)
	return nil
}

func clearTempFiles() {
	tmpDirs := []string{"./tmp_tspl", "./out_tspl", "/tmp/tspl_filter", "/tmp/tspl_pages", "/tmp/tspl_labels"}
	for _, dir := range tmpDirs {
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			os.Remove(filepath.Join(dir, f.Name()))
		}
	}
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
	// argv[0] pode ser o path completo ou apenas o nome
	// Para backend CUPS, pode ser "tspl:/dev/usb/lp5" (URI)
	arg0 := os.Args[0]

	// Se argv[0] contém ":" é provavelmente um URI de backend (tspl:/dev/...)
	if strings.Contains(arg0, ":") && strings.HasPrefix(arg0, "tspl:") {
		return "backend"
	}

	// Extrair apenas o nome do executável (sem path)
	name := filepath.Base(arg0)
	name = strings.ToLower(name)

	// Detectar modo pelo nome do executável
	// Nomes suportados:
	//   - tspl-backend, tspl (backend CUPS)
	//   - tspl-filter, tspl-thermal (filtro CUPS)
	//   - tspldriver ou outros (CLI)

	// Backend: tspl-backend ou tspl (nome curto para backend CUPS)
	if name == "tspl-backend" || name == "tspl" {
		return "backend"
	}

	// Filter: tspl-filter ou tspl-thermal
	if name == "tspl-filter" || name == "tspl-thermal" {
		return "filter"
	}

	// Fallback: detectar por substring (compatibilidade)
	if strings.Contains(name, "backend") {
		return "backend"
	}
	if strings.Contains(name, "filter") || strings.Contains(name, "thermal") {
		return "filter"
	}

	// Se temos 6+ argumentos e argv[1] é numérico, provavelmente é filtro CUPS
	// MAS só se argv[0] não for um URI (já tratado acima)
	if len(os.Args) >= 6 && !strings.Contains(arg0, ":") {
		if _, err := strconv.Atoi(os.Args[1]); err == nil {
			return "filter"
		}
	}

	return "cli"
}

// ----------------- main ------------------------------------------------------
func main() {
	// Detectar modo ANTES de flag.Parse() para evitar consumir argumentos do CUPS backend
	autoMode := detectMode()

	mode := flag.String("mode", autoMode, "mode: cli|filter|backend (auto-detected by executable name if empty)")
	dpi := flag.Int("dpi", 0, "override dpi")
	width := flag.Float64("width", 0, "label width mm override")
	height := flag.Float64("height", 0, "label height mm override")
	margin := flag.Float64("margin", 0, "margin mm override")
	gap := flag.Float64("gap", 0, "gap mm override")
	delay := flag.Int("delay", 0, "delay ms override")

	// Para backend e filter, não fazer flag.Parse() pois os argumentos são do CUPS
	var args []string
	var finalMode string

	if autoMode == "backend" || autoMode == "filter" {
		// Não chamar flag.Parse() - os argumentos são do protocolo CUPS
		finalMode = autoMode
		args = os.Args[1:]
	} else {
		flag.Parse()
		// usar o modo detectado ou o modo fornecido via flag
		finalMode = autoMode
		if *mode != "" && *mode != autoMode {
			finalMode = *mode
		}
		args = flag.Args()

		// apply CLI overrides (só no modo CLI)
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
	}

	recalcPixels()

	// CUPS backend exit codes:
	// 0 = CUPS_BACKEND_OK
	// 1 = CUPS_BACKEND_FAILED (retry later)
	// 2 = CUPS_BACKEND_AUTH_REQUIRED (asks for auth - DO NOT USE!)
	// 3 = CUPS_BACKEND_HOLD (holds job)
	// 4 = CUPS_BACKEND_STOP (stops queue)
	// 5 = CUPS_BACKEND_CANCEL (cancels job)

	// route modes
	switch finalMode {
	case "filter":
		// CUPS filter mode: receives job-id user title copies options [filename]
		if err := modeFilter(os.Args); err != nil {
			logErr("filter error: %v", err)
			os.Exit(1) // CUPS_BACKEND_FAILED - will retry
		}
	case "backend":
		if err := modeBackend(os.Args); err != nil {
			logErr("backend error: %v", err)
			os.Exit(1) // CUPS_BACKEND_FAILED - will retry
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
			os.Exit(1)
		}
	}
}
