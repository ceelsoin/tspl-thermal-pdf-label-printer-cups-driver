# TSPL Driver for Thermal Printers

Complete driver for thermal printers using TSPL language (TSC, Zebra, etc.) with CUPS support and CLI mode.

## Demo

![Driver in action](./assets/running.gif)

## Features

- **Two printing modes**:
    - **SLICE MODE**: Slices A4 PDF into 4 labels of 10x15cm (2x2 grid)
    - **FULL PAGE MODE**: Prints entire page according the preview
- Automatic blank page detection (< 10% content)
- Full CUPS integration
- Browser preview (Chrome/Firefox)
- Multiple size support (A4, 4x6, 3x5, 2x4)
- Automatic margin and proportion adjustment
- Retry/backoff for transient USB errors
- CLI mode for direct use without CUPS

## Requirements

- Go 1.16+
- CUPS (Common Unix Printing System)
- Libraries: `go-fitz`, `imaging`
- Thermal printer with TSPL protocol

## Installation

### Automatic installation (recommended)

```bash
sudo ./install-cups-driver.sh
```

### Manual installation

```bash
# 1. Compile
go build -o tspldriver main.go

# 2. Install filter (755 permissions)
sudo cp tspldriver /usr/lib/cups/filter/tspl-filter
sudo chmod 755 /usr/lib/cups/filter/tspl-filter

# 3. Install backend (700 permissions - CRITICAL!)
# Backend MUST have 700 permissions or CUPS will ask for authentication
sudo cp tspldriver /usr/lib/cups/backend/tspl
sudo chmod 700 /usr/lib/cups/backend/tspl
sudo chown root:root /usr/lib/cups/backend/tspl

# 4. Copy PPD
sudo mkdir -p /usr/share/ppd/custom
sudo cp tspl-thermal.ppd /usr/share/ppd/custom/

# 5. Restart CUPS
sudo systemctl restart cups

# 6. Add printer
sudo lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P /usr/share/ppd/custom/tspl-thermal.ppd
```

### Using Makefile

```bash
# Build and install
sudo make install

# Add printer
sudo make add-printer

# Print test
make print-test

# See all commands
make help
```

## Usage Modes

### SLICE MODE - Automatic Slicing

When you select **PageSize=A4** in Chrome/Firefox:

```
A4 PDF (210x297mm)
        ↓
2x2 Grid (4 labels)
        ↓
4 × 10x15cm labels
```

**Features:**
- Automatically slices into 4 labels
- Ignores labels with < 10% content
- Applies safety margins
- Ideal for: Multiple Shopee labels, Mercado Livre, etc.

**Example:**
```bash
lp -d TSPLPrinter -o PageSize=A4 shopee-labels.pdf
```

### FULL PAGE MODE - Full Page

When you select **Label4x6**, **Label3x5** or **Label2x4**:

```
PDF (any size)
        ↓
Proportional resizing
        ↓
Single label
```

**Features:**
- Prints exactly what appears in preview
- No slicing - respecting proportions
- Resizes to selected size
- Ideal for: Pre-formatted labels, QR codes, custom designs

**Example:**
```bash
lp -d TSPLPrinter -o PageSize=Label4x6 single-label.pdf
```

## CUPS Usage

### Print from Chrome/Firefox

1. Open PDF in browser
2. Ctrl+P (Print)
3. Select "TSPLPrinter"
4. Choose size:
     - **A4 (4 labels per sheet)** → SLICE MODE
     - **4x6 Label (100x150mm)** → FULL PAGE MODE
     - **3x5 Label (76x127mm)** → FULL PAGE MODE
     - **2x4 Label (50x100mm)** → FULL PAGE MODE
5. Click "Print"

### Print via command line

```bash
# SLICE MODE - A4 PDF with multiple labels
lp -d TSPLPrinter -o PageSize=A4 labels.pdf

# FULL PAGE MODE - Single 10x15cm label
lp -d TSPLPrinter -o PageSize=Label4x6 label.pdf

# With additional options
lp -d TSPLPrinter -o PageSize=Label4x6 -o Resolution=300dpi label.pdf
```

## CLI Mode (Without CUPS)

To use the driver directly without CUPS:

```bash
# Basic syntax
./tspldriver <pdf> <device> [options]

# Example
./tspldriver labels.pdf /dev/usb/lp4

# With custom options
./tspldriver --dpi=203 --width=100 --height=150 --margin=2 --gap=2 labels.pdf /dev/usb/lp4
```

**Available options:**
- `--dpi=<value>`: DPI (default: 200)
- `--width=<mm>`: Label width in mm (default: 100)
- `--height=<mm>`: Label height in mm (default: 150)
- `--margin=<mm>`: Margin in mm (default: 2)
- `--gap=<mm>`: Gap between labels in mm (default: 2)
- `--delay=<ms>`: Delay between labels in ms (default: 200)

## Settings

### Supported Page Sizes

| PageSize | Dimensions | Mode | Use |
|----------|-----------|------|-----|
| **A4** | 210x297mm | SLICE | Multiple labels (2x2 grid) |
| **Label4x6** | 100x150mm | FULL PAGE | Single label |
| **Label3x5** | 76x127mm | FULL PAGE | Single label |
| **Label2x4** | 50x100mm | FULL PAGE | Single label |

### Resolutions

- **203 DPI** (default) - Compatible with most thermal printers
- **300 DPI** - Higher quality (check printer support)

## Troubleshooting

### Printer won't print

```bash
# Check status
lpstat -p TSPLPrinter

# Enable printer
sudo cupsenable TSPLPrinter
sudo cupsaccept TSPLPrinter

# View logs
sudo tail -f /var/log/cups/error_log
```

### Device not found

```bash
# List USB devices
ls -la /dev/usb/lp*

# If empty, check dmesg
dmesg | grep usb

# Add user to lp group
sudo usermod -aG lp $USER
```

### Preview doesn't show A4 option

1. Clear browser cache
2. Restart CUPS: `sudo systemctl restart cups`
3. Check if PPD is updated in `/etc/cups/ppd/TSPLPrinter.ppd`

### Labels cut off

- For SLICE MODE: Adjust margins in code (`MARGIN_MM`, `SAFE_MARGIN_RIGHT_MM`)
- For FULL PAGE: Use correct PageSize matching physical label size

### CUPS asks for authentication / Job pauses

This happens when the backend has wrong permissions:

```bash
# Check backend permissions
ls -la /usr/lib/cups/backend/tspl

# Should show: -rwx------ (700), owned by root:root
# If it shows 755, fix it:
sudo chmod 700 /usr/lib/cups/backend/tspl
sudo chown root:root /usr/lib/cups/backend/tspl
sudo systemctl restart cups
```

### Job completes but nothing prints

Check the CUPS logs for errors:

```bash
sudo tail -50 /var/log/cups/error_log | grep -E "\[Job"
```

Common issues:
- Backend not receiving data from filter
- USB device permissions
- Wrong device path (check with `ls /dev/usb/lp*`)

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    PDF Input                        │
└─────────────────────┬───────────────────────────────┘
                      │
                      ▼
                 ┌────────────────────────┐
                 │    CUPS Spooler       │
                 └────────────┬───────────┘
                      │
                      ▼
                 ┌────────────────────────┐
                 │   tspl-filter         │◄──── PageSize option
                 │   (PDF → TSPL)        │
                 └────────────┬───────────┘
                      │
                 ┌────────────▼───────────┐
                 │   SLICE_MODE Check    │
                 └────────────┬───────────┘
                      │
                 ┌────────────▼────────────────┐
                 │                             │
        ┌────▼─────┐              ┌───────▼────────┐
        │  A4 Mode │              │  Label Mode    │
        │  (SLICE) │              │  (FULL PAGE)   │
        └────┬─────┘              └───────┬────────┘
             │                             │
        ┌────▼──────┐              ┌──────▼────────┐
        │ Grid 2x2  │              │ Resize to fit │
        │ 4 labels  │              │ Single label  │
        └────┬──────┘              └───────┬───────┘
             │                             │
             └──────────┬──────────────────┘
                        ▼
                 ┌────────────────────────┐
                 │    TSPL Commands      │
                 │ (SIZE, GAP, BITMAP)   │
                 └────────────┬───────────┘
                      │
                      ▼
                 ┌────────────────────────┐
                 │    tspl Backend       │
                 │   (TSPL → Device)     │
                 └────────────┬───────────┘
                      │
                      ▼
                 ┌────────────────────────┐
                 │    /dev/usb/lpX       │
                 │  (Thermal Printer)    │
                 └────────────────────────┘
```

### Components

1. **Filter** (`/usr/lib/cups/filter/tspl-filter`)
     - Converts PDF to TSPL
     - Detects PageSize and activates appropriate mode
     - Generates commands: SIZE, GAP, BITMAP, PRINT
     - Permissions: **755** (readable/executable by all)

2. **Backend** (`/usr/lib/cups/backend/tspl`)
     - Sends TSPL to printer
     - Manages retry/backoff for USB
     - 512-byte chunking with delay
     - Permissions: **700** (root only - CRITICAL!)
     - ⚠️ If permissions are 755, CUPS will ask for authentication!

3. **PPD** (`/usr/share/ppd/custom/tspl-thermal.ppd`)
     - Defines printer capabilities
     - Declares page sizes and resolutions

## Additional Documentation

- [CUPS-README.md](CUPS-README.md) - Detailed CUPS installation and usage guide
- [main.go](main.go) - Commented source code

## License

MIT License

## Author

Celso Inacio 

---

**Note:** For optimal results, use A4 for multiple labels (batch printing) and Label4x6 for individual pre-formatted labels.

