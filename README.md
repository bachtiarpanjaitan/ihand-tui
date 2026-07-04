# Ihand TUI — AI Chat in Your Terminal

Chat dengan AI langsung dari terminal. Tampilan full-screen, dukungan markdown, file operations, dan bisa pakai **OpenAI**, **Anthropic**, **Ollama**, atau provider OpenAI-compatible lainnya (Groq, Together AI, DeepSeek, dll).

![Go](https://img.shields.io/badge/go-1.23+-blue) ![License](https://img.shields.io/badge/license-MIT-green)

---

## Quick Start

### 1. Install

```bash
git clone https://github.com/bachtiarpanjaitan/ihand-tui.git
cd ihand-tui
go build -o ihand-tui .
```

### 2. Konfigurasi

Edit `settings.json`, sesuaikan dengan provider yang dipakai:

```json
{
  "llm": {
    "provider": "openai",
    "model": "gpt-4o",
    "api_key": "sk-proj-...",
    "base_url": ""
  }
}
```

### 3. Jalankan

```bash
./ihand-tui
```

Selesai. Mulai ketik pertanyaan, Enter untuk kirim.

---

## Konfigurasi Provider

### OpenAI

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

Pastikan Ollama sudah terinstall dan model sudah di-pull:

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

Provider manapun yang punya endpoint `/v1/chat/completions` bisa dipakai. Tinggal isi `base_url`:

```json
// Groq
{
  "llm": {
    "provider": "openai",
    "model": "llama-3.1-70b-versatile",
    "api_key": "gsk_...",
    "base_url": "https://api.groq.com/openai/v1"
  }
}

// Together AI
{
  "llm": {
    "provider": "openai",
    "model": "meta-llama/Llama-3.1-70B-Instruct",
    "api_key": "tgp_...",
    "base_url": "https://api.together.xyz/v1"
  }
}

// DeepSeek
{
  "llm": {
    "provider": "openai",
    "model": "deepseek-chat",
    "api_key": "sk-...",
    "base_url": "https://api.deepseek.com/v1"
  }
}
```

---

## Penggunaan

### Chat

Ketik pertanyaan atau instruksi, lalu Enter. AI akan merespon dengan markdown (tabel, code block, list, dll akan dirender dengan baik di terminal).

```
▸ jelaskan apa itu goroutine
✓ [1.2s · ~150 token]
─────────────────────────────────────────
Goroutine adalah lightweight thread...
─────────────────────────────────────────
```

### Mode Operasi

Ihand TUI punya 4 mode yang bisa diganti kapan saja. Mode menentukan bagaimana AI bekerja:

| Mode | Command | Perilaku |
|------|---------|----------|
| **Chat** | `/chat` | Percakapan normal, bisa pakai semua tools |
| **Plan** | `/plan` | Analisis & perencanaan. Hanya bisa **read**, tidak bisa write file. AI akan membaca kode dan membuat rencana langkah-demi-langkah |
| **Edit** | `/edit` | Implementasi langsung. AI akan mengerjakan perubahan tanpa bertanya — langsung tulis/edit file |
| **Auto** | `/auto` | Otonom penuh. AI merencanakan dan mengeksekusi multi-step sendiri (max 16 iterasi) |

Mode saat ini ditampilkan di header dengan warna berbeda: 🔵 Chat, 🟠 Plan, 🟢 Edit, 🔴 Auto.

### Slash Commands

Ketik `/` untuk melihat perintah yang tersedia:

| Command | Fungsi |
|---------|--------|
| `/chat` | Mode percakapan normal |
| `/plan` | Mode analisis & rencana (read-only) |
| `/edit` | Mode implementasi & edit file |
| `/auto` | Mode otonom (multi-step) |
| `/exit` | Keluar aplikasi |
| `/clear` | Reset percakapan |
| `/stats` | Lihat statistik session |
| `/help` | Tampilkan bantuan |

Tekan `Tab` untuk cycling antar suggestion, Enter untuk memilih.

### File Operations

AI bisa membaca, menulis, dan me-list file di direktori yang diizinkan:

```
▸ tulis file halo.txt isinya Hello World
🔧 write_file({"path":"halo.txt","content":"Hello World"}) → File berhasil ditulis (11 bytes)
✓ File halo.txt berhasil dibuat!

▸ baca isi file halo.txt
🔧 read_file({"path":"halo.txt"}) → {"content":"Hello World"}
✓ Isi file halo.txt adalah "Hello World"

▸ file apa saja yang ada di sini?
🔧 list_files({"path":"."}) → [halo.txt, main.go, ...]
```

> **Keamanan:** Tools dibatasi hanya dalam direktori yang diizinkan. Akses `../` ditolak, read dibatasi 1MB.

### Keyboard Shortcuts

| Key | Fungsi |
|-----|--------|
| `Enter` | Kirim pesan |
| `Ctrl+J` | Baris baru (multiline input) |
| `Tab` | Cycling suggestion (saat autocomplete muncul) |
| `Shift+Tab` | Ganti mode: Chat → Plan → Edit → Auto |
| `Ctrl+C` / `Ctrl+D` | Keluar |
| `Ctrl+L` | Scroll ke atas |
| `↑` `↓` | Scroll viewport per baris |
| `PgUp` `PgDn` | Scroll per halaman |
| `Home` `End` | Lompat ke awal / akhir |
| `Mouse wheel` | Scroll |

---

## Flag CLI

| Flag | Default | Fungsi |
|------|---------|--------|
| `--config` | `settings.json` | Path ke file konfigurasi |
| Argumen ke-1 | `.` | Direktori untuk file operations |

```bash
# Pakai config di lokasi lain
./ihand-tui --config ~/.config/ihand-tui.json

# Batasi file operations ke folder tertentu
./ihand-tui ~/projects
```

---

## License

MIT
