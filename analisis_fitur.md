# Analisis Fitur Auto-read .md File di Root Project

**Pertanyaan:** Apakah aplikasi TUI ini sudah bisa membaca misalnya apabila ada .md file di root project maka otomatis baca terlebih dahulu dan dimasukkan di dalam context?

**Kesimpulan:** **TIDAK**, aplikasi TUI ini belum memiliki fitur otomatis membaca file `.md` (seperti `README.md`) di root project dan memasukkannya ke dalam context percakapan.

## Bukti dari Kode Sumber

1. **Tidak ada inisialisasi context dari file di root**:
   - Fungsi `initialModel()` dan `initModel()` di `model.go` hanya menginisialisasi UI, tool list, dan trust mechanism. Tidak ada logika yang mencari file `.md` atau `README*` di root.
   - Fungsi `welcomeMessage()` hanya menampilkan teks sambutan statis, bukan konten dari file eksternal.

2. **System prompt tidak memuat konten file**:
   - Pencarian kata kunci "system prompt", "context file", "root file", "README", dan "markdown" (di luar komentar formatting tool) tidak menemukan hasil yang relevan.
   - Satu-satunya penyebutan "markdown" di `chat.go:873` hanya untuk membersihkan formatting nama tool, bukan terkait pembacaan file markdown.

3. **LLM harus membaca file secara eksplisit**:
   - Aplikasi sudah memiliki tools seperti `read_file`, `find_files`, `search_text`, `list_files` yang bisa digunakan oleh LLM **jika diminta**. Namun, fitur otomatis membaca file `.md` di root dan langsung memasukkannya ke dalam system prompt **belum ada**.

## Catatan

- Fitur ini dapat ditambahkan dengan memodifikasi `initModel()` atau bagian system prompt builder untuk mencari file seperti `README.md` di `allowedDir` dan menambahkannya ke dalam prompt awal.
