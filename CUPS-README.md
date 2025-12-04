# Driver CUPS para Impressoras Térmicas TSPL

Driver CUPS para impressoras térmicas que usam linguagem TSPL (como TSC, Zebra, etc.).

## Características

- ✅ Conversão automática de PDF A4 para etiquetas 10x15cm (4 por página)
- ✅ Modo de impressão inteligente: fatiamento em grid 2x2 (A4) ou página inteira (tamanhos de label)
- ✅ Detecção inteligente de páginas vazias (ignora se < 10% de conteúdo)
- ✅ Suporte a impressão via CUPS
- ✅ Preview no navegador via interface web do CUPS
- ✅ Suporte a múltiplos tamanhos de etiqueta (A4, 4x6, 3x5, 2x4)
- ✅ Ajuste automático de margens

## Instalação

### 1. Compilar e instalar

```bash
sudo ./install-cups-driver.sh
```

### 2. Adicionar impressora

**Via interface web** (recomendado):
```bash
# Abrir navegador em:
http://localhost:631

# Seguir:
# 1. Administration > Add Printer
# 2. Selecionar dispositivo USB (ex: /dev/usb/lp5)
# 3. Escolher driver: "TSPL Thermal Label Printer"
# 4. Definir como padrão (opcional)
```

**Via linha de comando**:
```bash
# Adicionar impressora
sudo lpadmin -p TSPLPrinter -E -v tspl:/dev/usb/lp5 -P /usr/share/ppd/custom/tspl-thermal.ppd

# Definir como padrão (opcional)
sudo lpadmin -d TSPLPrinter
```

## Uso

### Modos de Impressão

O driver suporta dois modos de operação baseados no tamanho de página selecionado:

#### 1. **SLICE MODE** (Modo de Fatiamento) - Página A4
Quando você seleciona **PageSize=A4**:
- ✂️ O driver fatia a página A4 em um grid 2x2 (4 etiquetas de 10x15cm)
- 🔍 Detecta automaticamente etiquetas vazias (< 10% de conteúdo) e as ignora
- 📏 Aplica margens de segurança para evitar cortes
- 📄 Ideal para: PDFs A4 com layout de múltiplas etiquetas

**Exemplo de uso:**
```bash
lp -d TSPLPrinter -o PageSize=A4 etiquetas-multiplas.pdf
```

#### 2. **FULL PAGE MODE** (Modo Página Inteira) - Tamanhos de Label
Quando você seleciona **Label4x6**, **Label3x5** ou **Label2x4**:
- 📄 O driver imprime a página inteira como aparece no preview
- 🎯 Não há fatiamento - respeita exatamente o que você vê no navegador
- 📐 Redimensiona proporcionalmente para o tamanho de etiqueta selecionado
- 🖼️ Ideal para: PDFs já formatados para uma etiqueta específica

**Exemplo de uso:**
```bash
lp -d TSPLPrinter -o PageSize=Label4x6 etiqueta-unica.pdf
```

### Imprimir via CUPS

```bash
# Modo SLICE (A4 → 4 etiquetas)
lp -d TSPLPrinter -o PageSize=A4 arquivo-a4.pdf

# Modo FULL PAGE (página inteira em etiqueta 10x15cm)
lp -d TSPLPrinter -o PageSize=Label4x6 etiqueta.pdf

# Com resolução customizada
lp -d TSPLPrinter -o PageSize=Label4x6 -o Resolution=300dpi etiqueta.pdf
```

### Imprimir via CLI (modo direto)

```bash
# Modo CLI (sem CUPS)
./tspldriver --dpi=203 --width=100 --height=150 --margin=2 --gap=2 arquivo.pdf /dev/usb/lp5
```

### Preview no navegador

1. Acesse http://localhost:631/printers/TSPLPrinter
2. Clique em "Maintenance" > "Print Test Page"
3. Ou use qualquer aplicativo que suporte impressão no Chrome/Firefox

## Configurações

### Tamanhos de etiqueta suportados

- **A4** (210x297mm) - Modo SLICE: fatia em 4 etiquetas 10x15cm
- **4x6** (100x150mm) - Modo FULL PAGE: imprime página inteira
- **3x5** (76x127mm) - Modo FULL PAGE: imprime página inteira
- **2x4** (50x100mm) - Modo FULL PAGE: imprime página inteira

### Resoluções

- 203 DPI (padrão)
- 300 DPI

### Opções de impressão

```bash
# DPI personalizado
lp -d TSPLPrinter -o Resolution=300dpi arquivo.pdf

# Tamanho personalizado
lp -d TSPLPrinter -o PageSize=Label3x5 arquivo.pdf
```

## Troubleshooting

### Impressora não aparece

```bash
# Verificar se CUPS está rodando
systemctl status cups

# Reiniciar CUPS
sudo systemctl restart cups

# Verificar dispositivo USB
ls -la /dev/usb/lp*
```

### Permissões

```bash
# Adicionar usuário ao grupo lp
sudo usermod -aG lp $USER

# Recarregar grupos (ou fazer logout/login)
newgrp lp
```

### Verificar logs

```bash
# Logs do CUPS
tail -f /var/log/cups/error_log

# Logs do filtro
sudo journalctl -u cups -f
```

### Remover instalação

```bash
# Remover impressora
sudo lpadmin -x TSPLPrinter

# Remover arquivos
sudo rm -f /usr/lib/cups/filter/tspl-thermal
sudo rm -f /usr/lib/cups/backend/tspl
sudo rm -f /usr/share/ppd/custom/tspl-thermal.ppd

# Reiniciar CUPS
sudo systemctl restart cups
```

## Arquitetura

### Pipeline CUPS

```
PDF → CUPS → Filtro TSPL → Backend TSPL → Impressora
```

### Fluxo de Processamento

#### Modo SLICE (PageSize=A4)
```
PDF A4 (210x297mm)
   ↓
Renderização para PNG @ 203 DPI
   ↓
Fatiamento em grid 2x2
   ↓
4 etiquetas 10x15cm (100x150mm)
   ↓
Detecção de páginas vazias (<10% conteúdo)
   ↓
Conversão para TSPL bitmap
   ↓
Envio para /dev/usb/lpX
```

#### Modo FULL PAGE (PageSize=Label4x6/3x5/2x4)
```
PDF (qualquer tamanho)
   ↓
Renderização para PNG @ 203 DPI
   ↓
Redimensionamento proporcional
   ↓
Conversão para TSPL bitmap
   ↓
Envio para /dev/usb/lpX
```

### Componentes

- **Filtro** (`/usr/lib/cups/filter/tspl-thermal`): Converte PDF → TSPL
  - Detecta PageSize das opções CUPS
  - Ativa SLICE_MODE para A4, FULL PAGE para labels
  - Gera comandos TSPL (SIZE, GAP, BITMAP, PRINT)

- **Backend** (`/usr/lib/cups/backend/tspl`): Envia TSPL → dispositivo
  - Lê TSPL do filtro via stdin
  - Gerencia retry/backoff para erros USB transientes
  - Escreve para `/dev/usb/lpX` com chunking de 512 bytes

- **PPD** (`/usr/share/ppd/custom/tspl-thermal.ppd`): Define capacidades
  - PageSize: A4, Label4x6, Label3x5, Label2x4
  - Resolution: 203dpi, 300dpi
  - cupsFilter: application/pdf → tspl-thermal

## Desenvolvimento

### Modos de operação

O driver suporta 3 modos:

1. **CLI**: Uso direto via linha de comando
2. **Filter**: Modo filtro CUPS (recebe PDF, converte para TSPL)
3. **Backend**: Modo backend CUPS (envia TSPL para impressora)

### Testar sem instalar

```bash
# Compilar
go build -o tspldriver main.go

# Modo filter com SLICE (A4 → 4 etiquetas)
./tspldriver --mode=filter 1 user title 1 "PageSize=A4" arquivo-a4.pdf > output.tspl

# Modo filter com FULL PAGE (página inteira)
./tspldriver --mode=filter 1 user title 1 "PageSize=Label4x6" etiqueta.pdf > output.tspl

# Modo backend
cat output.tspl | ./tspldriver --mode=backend tspl:/dev/usb/lp5
```

## Licença

MIT License - veja LICENSE para detalhes
