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

# Compila o binário Go
echo "Compilando driver..."
go build -o tspldriver main.go

# Instala como filtro CUPS
echo "Instalando filtro CUPS..."
cp tspldriver "${FILTER_DIR}/${DRIVER_NAME}"
chmod 755 "${FILTER_DIR}/${DRIVER_NAME}"

# Instala como backend CUPS
echo "Instalando backend CUPS..."
cp tspldriver "${BACKEND_DIR}/tspl"
chmod 755 "${BACKEND_DIR}/tspl"

# Copia o PPD
echo "Copiando arquivo PPD..."
cp "${DRIVER_NAME}.ppd" "${PPD_DIR}/"
chmod 644 "${PPD_DIR}/${DRIVER_NAME}.ppd"

# Reinicia CUPS
echo "Reiniciando CUPS..."
systemctl restart cups

echo ""
echo "=== Instalação concluída! ==="
echo ""
echo "Para adicionar a impressora:"
echo "1. Via Web: http://localhost:631"
echo "   - Administration > Add Printer"
echo "   - Selecione: TSPL Thermal Label Printer"
echo ""
echo "2. Via linha de comando:"
echo "   lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P ${PPD_DIR}/${DRIVER_NAME}.ppd"
echo ""
echo "Para testar impressão:"
echo "   lp -d TSPLPrinter seu-arquivo.pdf"
echo ""
echo "Para visualizar preview no navegador:"
echo "   Acesse: http://localhost:631/printers/TSPLPrinter"
echo ""
echo ""