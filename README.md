# Ihand TUI — AI Chat in Your Terminal

Chat dengan AI langsung dari terminal. Full-screen TUI, rendering markdown, file operations, dan dukungan **OpenAI**, **Anthropic**, **Ollama**, atau provider OpenAI-compatible lainnya (Groq, Together AI, DeepSeek, dll).

![Go](https://img.shields.io/badge/go-1.23+-blue) ![License](https://img.shields.io/badge/license-MIT-green)

---

## Quick Start

### Install (macOS — Homebrew)

```bash
brew tap bachtiarpanjaitan/homebrew-tap
brew trust bachtiarpanjaitan/tap/ihand-tui   # required since Homebrew 4.10+
brew install ihand-tui
```

> **Trust error?** Homebrew 4.10+ mewajibkan trust untuk third-party tap.
> Ganti `brew trust` dengan satu perintah berikut jika gagal:
> ```bash
> export HOMEBREW_NO_REQUIRE_TAP_TRUST=1
> brew install bachtiarpanjaitan/tap/ihand-tui
> ```

### Install (macOS / Linux — curl)

```bash
curl -fsSL https://raw.githubusercontent.com/bachtiarpanjaitan/ihand-tui/master/scripts/install-remote.sh | bash
```

Setelah itu, jalankan dari mana saja:

```bash
ihand
```

> **No dependencies.** Download binary langsung dari GitHub Releases. Lihat [Install Methods](#install-methods) untuk alternatif lain.

### Konfigurasi

Saat pertama kali dijalankan, `ihand` otomatis membuat file config di lokasi berikut:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/ihand/settings.json` |
| Linux | `~/.config/ihand/settings.json` |
| Windows | `%APPDATA%/ihand/settings.json` |

Edit file tersebut sesuai provider yang dipakai:

```json
{
  "llm": {
    "provider": "openai",
    "model": "gpt-4o",
    "api_key": "sk-proj-...",
    "base_url": ""
  },
  "app": {
    "allowed_dir": ".",
    "session": "default"
  }
}
```

```bash
# Jalankan
ihand

# Atau dengan config custom
ihand --config ~/.config/ihand.json

# Batasi file operations ke folder tertentu
ihand ~/projects
```

---

## Konfigurasi Provider

### OpenAI Compatible

```json
{
  "llm": {
    "provider": "openai",
    "model": "gpt-4o",
    "api_key": "sk-proj-..."
  }
}
```

### Anthropic (Claude)

```json
{
  "llm": {
    "provider": "anthropic",
    "model": "claude-sonnet-5",
    "api_key": "sk-ant-..."
  }
}
```

### Ollama (local, gratis)

```bash
ollama pull llama3.2
```

```json
{
  "llm": {
    "provider": "ollama",
    "model": "llama3.2"
  }
}
```

### OpenAI-compatible (Groq, Together AI, DeepSeek, dll)

Provider manapun dengan endpoint `/v1/chat/completions`:

```json
// DeepSeek via Anthropic endpoint
{
  "llm": {
    "provider": "anthropic",
    "model": "deepseek-v4-pro",
    "api_key": "sk-...",
    "base_url": "https://api.deepseek.com/anthropic"
  }
}

// Groq
{
  "llm": {
    "provider": "openai",
    "model": "llama-3.1-70b-versatile",
    "api_key": "gsk_...",
    "base_url": "https://api.groq.com/openai/v1"
  }
}
```

---

## Fitur

### 1. Empat Mode Operasi

Ganti mode kapan saja dengan `Shift+Tab` atau slash command:

| Mode | Command | Perilaku | Tools |
|------|---------|----------|-------|
| 💬 **Chat** | `/chat` | Percakapan normal, bisa pakai semua tools | read, write, list |
| 📋 **Plan** | `/plan` | Analisis & perencanaan. AI membaca kode dan membuat rencana. **Read-only** — tidak bisa write. | read, list |
| ✏️ **Edit** | `/edit` | Implementasi langsung. AI mengerjakan perubahan tanpa banyak tanya. | read, write, list |
| 🤖 **Auto** | `/auto` | Otonom penuh. AI merencanakan & mengeksekusi multi-step sendiri (max 16 iterasi). | read, write, list |

### 2. Slash Commands

Ketik `/` di input — autocomplete akan muncul. Tekan `Tab` untuk cycling.

| Command | Fungsi |
|---------|--------|
| `/chat` | Mode percakapan normal |
| `/plan` | Mode analisis & rencana (read-only) |
| `/edit` | Mode implementasi & edit file |
| `/auto` | Mode otonom multi-step |
| `/clear` | Reset percakapan |
| `/stats` | Statistik session (token, pesan, dll) |
| `/help` | Tampilkan bantuan |
| `/exit` | Keluar aplikasi |

### 3. @Mention File & Folder

Ketik `@` di input untuk autocomplete file/folder dari direktori kerja:

```
▸ tolong baca @cha           ← Tab untuk cycling
▸ tolong baca @chat.go       ← terpilih!
```

- **Case-insensitive** — `@cha` cocok dengan `chat.go`
- **Prefix match diprioritaskan** — file yang diawali query muncul duluan
- **Max 20 hasil**, depth 4 level
- Skip: `.git/`, `node_modules/`, hidden files, file >1MB
- Tekan `Tab` untuk cycling antar saran, saran yang dipilih menggantikan hanya bagian `@query`

### 4. File Operations

AI bisa membaca, menulis, dan me-list file. Tool output ditampilkan dengan format ringkas:

```
▸ buat file server.go dengan HTTP server sederhana

🔧 write_file({"path":"server.go","content":"..."}) → ✓ File berhasil ditulis: server.go (1024 bytes)
🔧 read_file({"path":"server.go"}) → ✓ Dibaca: server.go (1024 bytes)
🔧 list_files({"path":"."}) → ✓ 12 file/direktori di .
```

> **Keamanan:** Tools dibatasi hanya dalam `allowed_dir`. Path traversal (`../`) ditolak. Read dibatasi 1MB.

### 5. Markdown Rendering

Response AI di-render dengan Glamour (dark theme):
- **Code block** dengan syntax highlighting
- **Tabel**, list, heading
- **Bold**, italic, link
- Word wrapping otomatis sesuai lebar terminal

### 6. ReAct Loop dengan Real-time Update

Setiap langkah AI — thinking, tool call, hasil — ditampilkan **real-time** di chat. Tidak perlu menunggu seluruh proses selesai.

---

## Keyboard Shortcuts

| Key | Fungsi |
|-----|--------|
| `Enter` | Kirim pesan |
| `Ctrl+J` | Baris baru (multiline input) |
| `Tab` | Cycling suggestion (slash command / @mention) |
| `Shift+Tab` | Ganti mode: Chat → Plan → Edit → Auto |
| `Ctrl+C` / `Ctrl+D` | Keluar |
| `Ctrl+L` | Scroll viewport ke atas |
| `↑` `↓` | Scroll per baris |
| `PgUp` `PgDn` | Scroll per halaman |
| `Home` `End` | Lompat ke awal / akhir |
| `Mouse wheel` | Scroll |

---

## CLI Flags

| Flag | Default | Fungsi |
|------|---------|--------|
| `--config` | `settings.json` | Path ke file konfigurasi JSON |
| `--version` | — | Tampilkan versi |
| Argumen ke-1 | `.` | Direktori untuk file operations |

```bash
ihand --version                          # → ihand version 1.0.0
ihand --config ~/.config/ihand.json     # config custom
ihand ~/my-project                       # batasi ke folder tertentu
```

---

## Install Methods

### Homebrew (macOS / Linux)

```bash
brew tap bachtiarpanjaitan/homebrew-tap
brew trust bachtiarpanjaitan/tap/ihand-tui   # required since Homebrew 4.10+
brew install ihand-tui
```

Setelah itu jalankan dari mana saja:

```bash
ihand
```

**Troubleshooting — trust error:**

Homebrew 4.10+ mewajibkan trust untuk third-party tap. Jika `brew trust` gagal:

```bash
# Opsi 1: bypass trust check (sementara)
export HOMEBREW_NO_REQUIRE_TAP_TRUST=1
brew install bachtiarpanjaitan/tap/ihand-tui

# Opsi 2: trust semua formula dari tap
brew trust bachtiarpanjaitan/tap

# Opsi 3: untap jika tidak digunakan
brew untap bachtiarpanjaitan/homebrew-tap
```

> Untuk update: `brew upgrade ihand-tui`

### curl (macOS & Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/bachtiarpanjaitan/ihand-tui/master/scripts/install-remote.sh | bash
```

Skrip akan clone repo, build, dan install ke `/usr/local/bin/ihand`.

### make (dari source)

```bash
git clone https://github.com/bachtiarpanjaitan/ihand-tui.git
cd ihand-tui

make install      # build + install ke /usr/local/bin
make build        # build saja (binary: ./ihand)
make build-all    # cross-compile semua platform → dist/
make uninstall    # hapus dari /usr/local/bin
```

### Install script lokal

```bash
# macOS / Linux
bash scripts/install.sh

# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File scripts/install.ps1
```

### Build manual

```bash
git clone https://github.com/bachtiarpanjaitan/ihand-tui.git
cd ihand-tui
go build -o ihand .
./ihand
```

### Cross-compile (untuk distribusi)

```bash
make build-all
# Output di dist/:
#   ihand-windows-amd64.exe
#   ihand-darwin-amd64
#   ihand-darwin-arm64
#   ihand-linux-amd64
#   ihand-linux-arm64
```

---

## Uninstall

```bash
# macOS / Linux
sudo rm /usr/local/bin/ihand

# Atau via make (dari direktori proyek)
make uninstall

# Windows
rm -r %USERPROFILE%\AppData\Local\ihand
# Lalu hapus dari User PATH via System Settings
```

---

## Project Structure

```
ihand-tui/
├── main.go              # Entry point, flag parsing, init
├── model.go             # Data model, types, constructor
├── chat.go              # ReAct loop, tool parsing, display formatting
├── update.go            # Bubble Tea Update, key handling, @mention
├── view.go              # Bubble Tea View, conversation rendering, markdown
├── commands.go          # Slash commands & autocomplete
├── helpers.go           # Token counter, file suggestion search
├── styles.go            # Lipgloss style definitions
├── layout.go            # Layout calculation
├── welcome.go           # Welcome message
├── config.go            # JSON config loading
├── Makefile             # Build, install, cross-compile
├── scripts/
│   ├── install.sh       # Installer lokal (macOS/Linux)
│   ├── install-remote.sh # Installer via curl
│   └── install.ps1      # Installer Windows
├── internal/
│   ├── providers/       # LLM provider implementations
│   │   ├── openai.go
│   │   └── anthropic.go
│   └── tools/           # File operation tools
│       └── tools.go     # ReadFile, WriteFile, ListFiles
├── settings.json        # Konfigurasi (user-specific, di-gitignore)
└── go.mod
```

---

## License

MIT
