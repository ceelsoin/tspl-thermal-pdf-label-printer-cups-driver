.PHONY: all build install uninstall clean test add-printer remove-printer print-test status logs help

DRIVER_NAME = tspldriver
BACKEND_NAME = tspl
FILTER_NAME = tspl-filter

BACKEND_DIR = /usr/lib/cups/backend
FILTER_DIR  = /usr/lib/cups/filter
PPD_DIR     = /usr/share/ppd/custom

# PPD file name
PPD_FILE = tspl-thermal.ppd

BUILD_DIR = build

all: build

build:
	@echo "=== Building TSPL driver ==="
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(DRIVER_NAME) main.go
	@echo "Build complete: $(BUILD_DIR)/$(DRIVER_NAME)"

# ----------------------------------------------------------------------
# INSTALL
# ----------------------------------------------------------------------
install: build
	@echo "=== Installing TSPL CUPS driver (backend + filter + ppd) ==="
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root (sudo)"; exit 1; fi

	# BACKEND
	mkdir -p $(BACKEND_DIR)
	install -m 700 $(BUILD_DIR)/$(DRIVER_NAME) $(BACKEND_DIR)/$(BACKEND_NAME)
	ls -l $(BACKEND_DIR)/$(BACKEND_NAME)

	# FILTER
	mkdir -p $(FILTER_DIR)
	install -m 755 $(BUILD_DIR)/$(DRIVER_NAME) $(FILTER_DIR)/$(FILTER_NAME)
	ls -l $(FILTER_DIR)/$(FILTER_NAME)

	# PPD directory
	mkdir -p $(PPD_DIR)

	# Install PPD file
	@if [ -f "$(PPD_FILE)" ]; then \
		echo "Installing PPD file $(PPD_FILE)..."; \
		install -m 644 $(PPD_FILE) $(PPD_DIR)/$(PPD_FILE); \
		ls -l "$(PPD_DIR)/$(PPD_FILE)"; \
	else \
		echo "ERROR: PPD file '$(PPD_FILE)' not found!"; \
		exit 1; \
	fi

	# Set correct ownership and permissions
	# FILTER: 755 (readable/executable by all)
	chown root:lp /usr/lib/cups/filter/tspl-filter
	chmod 755 /usr/lib/cups/filter/tspl-filter

	# BACKEND: 700 (CRITICAL! Must be root-only or CUPS asks for authentication)
	chown root:root /usr/lib/cups/backend/tspl
	chmod 700 /usr/lib/cups/backend/tspl


	systemctl restart cups

	@echo ""
	@echo "=== Installation complete! ==="
	@echo "Backend installed to: $(BACKEND_DIR)/$(BACKEND_NAME) (mode 700)"
	@echo "Filter installed to:  $(FILTER_DIR)/$(FILTER_NAME) (mode 755)"
	@echo "PPD installed to:     $(PPD_DIR)/$(PPD_FILE)"
	@echo ""
	@echo "To add printer via lpadmin:"
	@echo "  sudo lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P $(PPD_DIR)/$(PPD_FILE)"
	@echo ""

# ----------------------------------------------------------------------
# UNINSTALL
# ----------------------------------------------------------------------
uninstall:
	@echo "=== Uninstalling TSPL CUPS driver ==="
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root (sudo)"; exit 1; fi
	rm -f $(BACKEND_DIR)/$(BACKEND_NAME)
	rm -f $(FILTER_DIR)/$(FILTER_NAME)
	rm -f $(PPD_DIR)/$(PPD_FILE)
	systemctl restart cups
	@echo "Uninstallation complete!"

# ----------------------------------------------------------------------
clean:
	@echo "=== Cleaning build files ==="
	rm -rf $(BUILD_DIR)
	rm -f *.o *.a
	@echo "Clean complete!"

# ----------------------------------------------------------------------
test:
	@echo "=== Testing driver logic ==="
	go test -v ./...

# ----------------------------------------------------------------------
# PRINTER MANAGEMENT
# ----------------------------------------------------------------------

add-printer:
	@echo "=== Adding TSPL printer ==="
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root"; exit 1; fi
	@read -p "Enter printer device (default /dev/usb/lp5): " dev ; \
	dev=$${dev:-/dev/usb/lp5}; \
	lpadmin -p TSPLPrinter -E -v tspl:$$dev -P $(PPD_DIR)/$(PPD_FILE)
	@echo "Printer 'TSPLPrinter' added!"

remove-printer:
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root"; exit 1; fi
	lpadmin -x TSPLPrinter
	@echo "Printer removed."

recreate:
	sudo make uninstall
	sudo make install
	sudo lpadmin -x TSPLPrinter 2>/dev/null || true
	sudo lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P $(PPD_DIR)/$(PPD_FILE)

print-test:
	@echo "=== Printing TSPL test label ==="
	echo "SIZE 100 mm,150 mm\nCLS\nTEXT 100,100,\"3\",0,1,1,\"TSPL OK\"\nPRINT 1\n" \
	| lp -d TSPLPrinter -

status:
	lpstat -p -d

logs:
	sudo tail -50 /var/log/cups/error_log

help:
	@echo "Targets:"
	@echo "  make build          - Build the driver"
	@echo "  make install        - Install backend + filter + PPD (requires sudo)"
	@echo "  make uninstall      - Remove driver (requires sudo)"
	@echo "  make add-printer    - Add printer interactively (requires sudo)"
	@echo "  make remove-printer - Remove printer (requires sudo)"
	@echo "  make recreate       - Reinstall driver and recreate printer"
	@echo "  make print-test     - Print test label"
	@echo "  make status         - Show printer status"
	@echo "  make logs           - Show CUPS logs"
	@echo "  make clean          - Clean build files"