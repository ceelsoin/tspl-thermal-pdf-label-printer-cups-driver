# CUPS Driver for TSPL Thermal Printers

CUPS driver for thermal printers that use TSPL language (such as TSC, Zebra, etc.).

## Features

- ✅ Automatic conversion from A4 PDF to 10x15cm labels (4 per page)
- ✅ Smart printing mode: 2x2 grid slicing (A4) or full page (label sizes)
- ✅ Smart blank page detection (ignores if < 10% content)
- ✅ CUPS printing support
- ✅ Browser preview via CUPS web interface
- ✅ Multiple label size support (A4, 4x6, 3x5, 2x4)
- ✅ Automatic margin adjustment

## Installation

### 1. Compile and install

```bash
sudo ./install-cups-driver.sh
```

### 2. Add printer

**Via web interface** (recommended):
```bash
# Open browser at:
http://localhost:631

# Follow:
# 1. Administration > Add Printer
# 2. Select USB device (e.g., /dev/usb/lp5)
# 3. Choose driver: "TSPL Thermal Label Printer"
# 4. Set as default (optional)
```

**Via command line**:
```bash
# Add printer
sudo lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P /usr/share/ppd/custom/tspl-thermal.ppd

# Set as default (optional)
sudo lpadmin -d TSPLPrinter
```

## Usage

### Printing Modes

The driver supports two operating modes based on the selected page size:

#### 1. **SLICE MODE** - A4 Page
When you select **PageSize=A4**:
- ✂️ The driver slices the A4 page into a 2x2 grid (4 labels of 10x15cm)
- 🔍 Automatically detects blank labels (< 10% content) and ignores them
- 📏 Applies safety margins to avoid cuts
- 📄 Ideal for: A4 PDFs with multiple label layouts

**Usage example:**
```bash
lp -d TSPLPrinter -o PageSize=A4 multiple-labels.pdf
```

#### 2. **FULL PAGE MODE** - Label Sizes
When you select **Label4x6**, **Label3x5**, or **Label2x4**:
- 📄 The driver prints the entire page as it appears in the preview
- 🎯 No slicing - respects exactly what you see in the browser
- 📐 Proportionally resizes to the selected label size
- 🖼️ Ideal for: PDFs already formatted for a specific label

**Usage example:**
```bash
lp -d TSPLPrinter -o PageSize=Label4x6 single-label.pdf
```

### Print via CUPS

```bash
# SLICE mode (A4 → 4 labels)
lp -d TSPLPrinter -o PageSize=A4 a4-file.pdf

# FULL PAGE mode (full page on 10x15cm label)
lp -d TSPLPrinter -o PageSize=Label4x6 label.pdf

# With custom resolution
lp -d TSPLPrinter -o PageSize=Label4x6 -o Resolution=300dpi label.pdf
```

### Print via CLI (direct mode)

```bash
# CLI mode (without CUPS)
./tspldriver --dpi=203 --width=100 --height=150 --margin=2 --gap=2 file.pdf /dev/usb/lp5
```

### Browser preview

1. Access http://localhost:631/printers/TSPLPrinter
2. Click "Maintenance" > "Print Test Page"
3. Or use any application that supports printing in Chrome/Firefox

## Configuration

### Supported label sizes

- **A4** (210x297mm) - SLICE mode: slices into 4 labels of 10x15cm
- **4x6** (100x150mm) - FULL PAGE mode: prints entire page
- **3x5** (76x127mm) - FULL PAGE mode: prints entire page
- **2x4** (50x100mm) - FULL PAGE mode: prints entire page

### Resolutions

- 203 DPI (default)
- 300 DPI

### Print options

```bash
# Custom DPI
lp -d TSPLPrinter -o Resolution=300dpi file.pdf

# Custom size
lp -d TSPLPrinter -o PageSize=Label3x5 file.pdf
```

## Troubleshooting

### Printer not showing up

```bash
# Check if CUPS is running
systemctl status cups

# Restart CUPS
sudo systemctl restart cups

# Check USB device
ls -la /dev/usb/lp*
```

### Permissions

```bash
# Add user to lp group
sudo usermod -aG lp $USER

# Reload groups (or logout/login)
newgrp lp
```

### CUPS asking for authentication / Job pauses

If CUPS asks for credentials or jobs pause immediately, it's likely a backend permission issue:

```bash
# Backend MUST have 700 permissions (root-only)
ls -la /usr/lib/cups/backend/tspl
# Should show: -rwx------ 1 root root

# If permissions are wrong (755), fix them:
sudo chmod 700 /usr/lib/cups/backend/tspl

# Restart CUPS
sudo systemctl restart cups
```

**Why?** CUPS considers backends with world-readable permissions (755) as "insecure" and requires authentication to use them.

### Check logs

```bash
# CUPS logs
tail -f /var/log/cups/error_log

# Filter logs
sudo journalctl -u cups -f
```

### Remove installation

```bash
# Remove printer
sudo lpadmin -x TSPLPrinter

# Remove files
sudo rm -f /usr/lib/cups/filter/tspl-filter
sudo rm -f /usr/lib/cups/backend/tspl
sudo rm -f /usr/share/ppd/custom/tspl-thermal.ppd

# Restart CUPS
sudo systemctl restart cups
```

## Architecture

### CUPS Pipeline

```
PDF → CUPS → TSPL Filter → TSPL Backend → Printer
```

### Processing Flow

#### SLICE Mode (PageSize=A4)
```
A4 PDF (210x297mm)
   ↓
Render to PNG @ 203 DPI
   ↓
Slice into 2x2 grid
   ↓
4 labels 10x15cm (100x150mm)
   ↓
Blank page detection (<10% content)
   ↓
Convert to TSPL bitmap
   ↓
Send to /dev/usb/lpX
```

#### FULL PAGE Mode (PageSize=Label4x6/3x5/2x4)
```
PDF (any size)
   ↓
Render to PNG @ 203 DPI
   ↓
Proportional resize
   ↓
Convert to TSPL bitmap
   ↓
Send to /dev/usb/lpX
```

### Components

- **Filter** (`/usr/lib/cups/filter/tspl-filter`): Converts PDF → TSPL (permissions: 755)
  - Detects PageSize from CUPS options
  - Activates SLICE_MODE for A4, FULL PAGE for labels
  - Generates TSPL commands (SIZE, GAP, BITMAP, PRINT)

- **Backend** (`/usr/lib/cups/backend/tspl`): Sends TSPL → device (permissions: 700)
  - Reads TSPL from filter via stdin
  - Manages retry/backoff for transient USB errors
  - Writes to `/dev/usb/lpX` with 512-byte chunking
  - **IMPORTANTE**: Permissão deve ser 700 (somente root), senão CUPS pede autenticação

- **PPD** (`/usr/share/ppd/custom/tspl-thermal.ppd`): Defines capabilities
  - PageSize: A4, Label4x6, Label3x5, Label2x4
  - Resolution: 203dpi, 300dpi
  - cupsFilter: application/pdf → tspl-filter

## Development

### Operating modes

The driver supports 3 modes:

1. **CLI**: Direct usage via command line
2. **Filter**: CUPS filter mode (receives PDF, converts to TSPL)
3. **Backend**: CUPS backend mode (sends TSPL to printer)

### Test without installing

```bash
# Compile
go build -o tspldriver main.go

# Filter mode with SLICE (A4 → 4 labels)
./tspldriver --mode=filter 1 user title 1 "PageSize=A4" a4-file.pdf > output.tspl

# Filter mode with FULL PAGE (entire page)
./tspldriver --mode=filter 1 user title 1 "PageSize=Label4x6" label.pdf > output.tspl

# Backend mode
cat output.tspl | ./tspldriver --mode=backend tspl:/dev/usb/lp5
```

## License

MIT License - see LICENSE for details

