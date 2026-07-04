# ihandai TUI — Terminal User Interface dengan Bubble Tea

Terminal full-screen yang interaktif dengan dukungan **input multiline, scrollable viewport, markdown rendering, autocomplete, dan file operations**. Dibangun dengan [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) dan [ihandai-go](https://github.com/bachtiarpanjaitan/ihandai-go).

![Go](https://img.shields.io/badge/go-1.26.1-blue) ![License](https://img.shields.io/badge/license-MIT-green)

---

## Fitur TUI

### 🖥️ Full-Screen Terminal
- **Alternate screen buffer** — mengambil alih seluruh layar terminal
- **Responsive resize** — tampilan otomatis menyesuaikan saat terminal di-resize
- **Scrollable viewport** — history percakapan bisa di-scroll dengan keyboard atau mouse wheel
- **Bottom input area** — textarea di bagian bawah dengan placeholder dan prompt custom

### 🎨 Styling & Rendering
- **Lipgloss styling** — warna adaptif (gelap & terang), borders, background, bold/dim
- **Markdown rendering** — syntax highlighting, tabel, list, code blocks via Glamour
- **Status bar** — menampilkan state (`⏳ Thinking...` / `✓ Ready`), token count, jumlah pesan
- **Header bar** — info model dan session ID

### ⌨️ Slash Command + Autocomplete
Ketik `/` memunculkan **suggestion bar** dengan command yang tersedia:

| Shortcut | Perilaku |
|----------|----------|
| `Tab` | Cycling antar suggestion, auto-fill textarea |
| Ketik | Filter suggestion (prefix match) |
| `Enter` | Kirim command yang dipilih |

| Command | Fungsi |
|---------|--------|
| `/exit` | Keluar aplikasi |
| `/clear` | Reset percakapan |
| `/stats` | Statistik session |
| `/help` | Bantuan |

### 🔧 Tools (File Operations)
Sistem dapat **menulis, membaca, dan me-list file** di direktori yang diizinkan:

| Tool | Fungsi |
|------|--------|
| `write_file` | Menulis konten ke file (auto-create direktori) |
| `read_file` | Membaca konten file (max 1MB) |
| `list_files` | Menampilkan daftar file dan folder |

Tool dipanggil otomatis via **ReAct loop** — parse output → eksekusi tool → observasi → ulangi hingga final answer (max 8 iterasi).

### 🔒 Keamanan
- **Path traversal protection** — menolak akses `../` dan path di luar direktori yang diizinkan
- **File size limit** — read dibatasi 1MB
- **No shell execution** — tools hanya melakukan file I/O

---

## Instalasi

### Prasyarat
- **Go** 1.23+
- **Ollama** terinstall dan running
- Model llama sudah di-pull:
  ```bash
  ollama pull llama3.2
  ```

### Build & Run

```bash
# Clone repository
git clone https://github.com/bachtiarpanjaitan/ihandai-tui.git
cd ihandai-tui

# Build
go build -o ihandai-tui .

# Run (default: direktori saat ini)
./ihandai-tui

# Run dengan direktori khusus untuk file operations
./ihandai-tui /path/ke/folder
```

Atau langsung:

```bash
go run .
```

---

## Penggunaan

### Input & Navigasi
```
▸ Ketik pesan di sini, lalu Enter untuk kirim
✓ [1.2s · ~150 token]
─────────────────────────────────────────
Jawaban akan muncul di viewport dengan markdown...
─────────────────────────────────────────
```

### File Operations
```
▸ tulis file halo.txt dengan isi Hello World
🔧 write_file({"path":"halo.txt","content":"Hello World"}) → File berhasil ditulis (11 bytes)
✓ File halo.txt berhasil dibuat!

▸ baca isi file halo.txt  
🔧 read_file({"path":"halo.txt"}) → {"content":"Hello World"}
✓ Isi file halo.txt adalah "Hello World"

▸ list file yang ada di sini
🔧 list_files({"path":"."}) → [halo.txt, main.go, tools.go, ...]
✓ Berikut daftar file: halo.txt, main.go, tools.go, ...
```

---

## Keyboard Shortcuts

| Key | Fungsi |
|-----|--------|
| `Enter` | Kirim input |
| `Tab` | Cycling autocomplete suggestion |
| `Ctrl+C` `Ctrl+D` | Keluar |
| `Ctrl+L` | Scroll viewport ke atas |
| `↑` `↓` | Scroll viewport per baris |
| `PgUp` `PgDn` | Scroll viewport per halaman |
| `Home` `End` | Lompat ke awal/akhir viewport |
| `Mouse wheel` | Scroll viewport |

---

## Arsitektur

```
main.go          — TUI (Bubble Tea v2): model, update, view, key handling, tool loop
tools.go         — File operation tools: WriteFileTool, ReadFileTool, ListFilesTool
go.mod / go.sum  — Dependencies
```

### Tech Stack

| Library | Versi | Fungsi |
|---------|-------|--------|
| `charm.land/bubbletea/v2` | v2.0.8 | TUI framework (Model-Update-View) |
| `charm.land/bubbles/v2` | v2.1.1 | textarea (input) + viewport (scrollable area) |
| `charm.land/lipgloss/v2` | v2.0.5 | Styling, colors, borders, layout |
| `charm.land/glamour/v2` | v2.0.1 | Markdown → ANSI-styled terminal text |
| `ihandai-go` | local | LLM provider, memory store, tools interface |

### Event Flow

```
KeyPressMsg  →  handleKeyPress  →  forward ke textarea/viewport
                                  →  Enter: kirim input, jalankan tool loop async
                                  →  Tab: cycling suggestion
                                  →  ↑↓: scroll viewport

WindowSizeMsg → recalcLayout → resize viewport + textarea + re-render markdown

llmResponseMsg (async) → tampilkan jawaban di viewport, GotoBottom
```

### Tool Loop
```
Input → build prompt + tool descriptions
      → LLM generates response
      → Parse output:
         "Action: write_file({...})" → Execute tool → feed result back → repeat (max 8x)
         "Final Answer: ..."         → Tampilkan ke user, simpan ke memory
```

---

## Konfigurasi

Ganti model Ollama di `main.go`:

```go
ai, err := ihandai.New(
    ihandai.WithLLM("ollama", llm.WithModel("llama3.2")),
    ihandai.WithMemory(store),
)
```

Direktori file operations bisa diatur via argument CLI:

```bash
./ihandai-tui /home/user/projects
```

---

## License

MIT
