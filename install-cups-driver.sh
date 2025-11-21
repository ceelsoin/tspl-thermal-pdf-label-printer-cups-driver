#!/bin/bash
# Script de instalação do driver CUPS para impressora térmica TSPL

set -e

DRIVER_NAME="tspl-thermal"
FILTER_DIR="/usr/lib/cups/filter"
PPD_DIR="/usr/share/ppd/custom"
BACKEND_DIR="/usr/lib/cups/backend"

echo "=== Instalando driver CUPS TSPL Thermal ==="

# Verifica se é root
if [ "$EUID" -ne 0 ]; then 
    echo "Por favor, execute como root (sudo)"
    exit 1
fi

# Cria diretórios necessários
mkdir -p "$FILTER_DIR"
mkdir -p "$PPD_DIR"
mkdir -p "$BACKEND_DIR"

# Compila o filtro Go
echo "Compilando filtro CUPS..."
go build -o "${FILTER_DIR}/${DRIVER_NAME}" ./cmd/cups-filter/

# Copia o PPD
echo "Copiando arquivo PPD..."
cp "./ppd/${DRIVER_NAME}.ppd" "${PPD_DIR}/"

# Define permissões corretas
chmod 755 "${FILTER_DIR}/${DRIVER_NAME}"
chmod 644 "${PPD_DIR}/${DRIVER_NAME}.ppd"

# Reinicia CUPS
echo "Reiniciando CUPS..."
systemctl restart cups

echo ""
echo "=== Instalação concluída! ==="
echo ""
echo "Para adicionar a impressora:"
echo "1. Acesse http://localhost:631"
echo "2. Administration > Add Printer"
echo "3. Selecione seu dispositivo USB (ex: /dev/usb/lp0)"
echo "4. Escolha o driver: TSPL Thermal Label Printer"
echo ""
echo "Ou via linha de comando:"
echo "  lpadmin -p thermal-printer -E -v file:/dev/usb/lp5 -P ${PPD_DIR}/${DRIVER_NAME}.ppd"
echo ""