.PHONY: all build install uninstall clean test add-printer remove-printer print-test status logs help

DRIVER_NAME = tspldriver
BACKEND_NAME = tspl
FILTER_NAME = tspl-filter

BACKEND_DIR = /usr/lib/cups/backend
FILTER_DIR  = /usr/lib/cups/filter
PPD_DIR     = /usr/share/ppd/custom

LOCAL_PPD = tspldriver.ppd

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

	# Install local PPD directly
	@if [ -f "$(LOCAL_PPD)" ]; then \
		echo "Installing local PPD file $(LOCAL_PPD)..."; \
		mkdir -p $(PPD_DIR); \
		install -m 644 $(LOCAL_PPD) $(PPD_DIR)/$(LOCAL_PPD); \
		ls -l "$(PPD_DIR)/"; \
	else \
		echo "Warning: local PPD '$(LOCAL_PPD)' not found in current directory $(PWD)!"; \
	fi

	# Install default PPD if it exists
	@if [ -f ppd/$(DRIVER_NAME).ppd ]; then \
		echo "Installing default PPD ppd/$(DRIVER_NAME).ppd ..."; \
		install -m 644 ppd/$(DRIVER_NAME).ppd $(PPD_DIR)/; \
		ls -l $(PPD_DIR)/$(DRIVER_NAME).ppd; \
	fi

	chown root:lp /usr/lib/cups/filter/tspl-filter
	chmod 755 /usr/lib/cups/filter/tspl-filter

	chown root:lp /usr/lib/cups/backend/tspl
	chmod 755 /usr/lib/cups/backend/tspl


	systemctl restart cups

	@echo ""
	@echo "=== Installation complete! ==="
	@echo "Backend installed to: $(BACKEND_DIR)/$(BACKEND_NAME)"
	@echo "Filter installed to:  $(FILTER_DIR)/$(FILTER_NAME)"
	@echo "PPD installed to:     $(PPD_DIR)"
	@echo ""
	@echo "To add printer via lpadmin:"
	@echo "  sudo lpadmin -p thermal-tspl -E -v tspl:/dev/usb/lp5 -P $(PPD_DIR)/$(LOCAL_PPD)"
	@echo ""

# ----------------------------------------------------------------------
# UNINSTALL
# ----------------------------------------------------------------------
uninstall:
	@echo "=== Uninstalling TSPL CUPS driver ==="
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root (sudo)"; exit 1; fi
	rm -f $(BACKEND_DIR)/$(BACKEND_NAME)
	rm -f $(FILTER_DIR)/$(FILTER_NAME)
	rm -f $(PPD_DIR)/$(LOCAL_PPD)
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
	@echo "=== Adding TSPL test printer ==="
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root"; exit 1; fi
	@read -p "Enter printer device (default /dev/usb/lp5): " dev ; \
	dev=$${dev:-/dev/usb/lp5}; \
	lpadmin -p tspl-test -E -v tspl:$$dev -P $(PPD_DIR)/$(DRIVER_NAME).ppd
	@echo "Printer 'tspl-test' added!"

remove-printer:
	@if [ "$$(id -u)" -ne 0 ]; then echo "Error: Must run as root"; exit 1; fi
	lpadmin -x tspl-test
	@echo "Printer removed."

recreate:
	sudo make uninstall
	sudo make install
	sudo lpadmin -x tspl-test
	sudo lpadmin -p tspl-test -E -v tspl:/dev/usb/lp5 -P /usr/share/ppd/custom/tspldriver.ppd

print-test:
	@echo "=== Printing TSPL test label ==="
	echo "SIZE 100 mm,150 mm\nCLS\nTEXT 100,100,\"3\",0,1,1,\"TSPL OK\"\nPRINT 1\n" \
	| lp -d tspl-test -

status:
	lpstat -p -d

logs:
	sudo tail -50 /var/log/cups/error_log

help:
	@echo "Targets:"
	@echo "  make build          - Build the driver"
	@echo "  make install        - Install backend + filter + ppd"
	@echo "  make uninstall      - Remove driver"
	@echo "  make add-printer    - Add test printer"
	@echo "  make print-test     - Print test label"
	@echo "  make logs           - Show CUPS logs"