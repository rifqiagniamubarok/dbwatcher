# IDEAS

Parking lot untuk fitur dan ide yang muncul selama development tapi **tidak masuk MVP**. Tujuannya supaya fokus tetap di PLAN.md tanpa kehilangan ide menarik.

Format:
```
## [tanggal] judul ide
Konteks: kenapa muncul
Deskripsi: apa fungsinya
Kategori: feature | refactor | tooling | docs
Effort estimate: kecil | sedang | besar
```

---

## Backlog ide (post-MVP)

### Session correlation (sudah di PLAN.md v0.2)
Tampilkan perubahan dikelompokkan per koneksi/session. Developer bisa lihat "satu API call menghasilkan 5 perubahan ini".

### SDK untuk test integration (sudah di PLAN.md v0.3)
Library Node/Python yang bisa di-import di test code untuk assert perubahan DB. Contoh:
```javascript
expect(await dbwatch.changesSince(startTime))
  .toContainInsert({ table: 'orders' });
```

### MCP server (sudah di PLAN.md v0.4)
Expose DBWatch sebagai MCP server. AI agent (Claude Code, dll) bisa connect dan query perubahan database sebagai bagian dari verifikasi code.

### Replay timeline
Slider waktu di TUI/Web untuk scrub balik dan lihat keadaan event di waktu tertentu.

### Query result snapshot
Sebelum operasi, simpan snapshot hasil query tertentu. Setelah operasi, tampilkan diff hasil query yang sama.

### Export & share
Export event session ke file (JSON, CSV) untuk attach ke bug report.

### Visual diff untuk JSON column
Kalau ada kolom JSONB, tampilkan diff struktur JSON-nya, bukan cuma string compare.

---

## Template untuk ide baru

Saat ada ide muncul saat development, tambahkan di sini dengan format:

```
## [YYYY-MM-DD] Judul singkat
Konteks: situasi yang memicu ide ini
Deskripsi: apa yang akan dibangun
Kategori: feature
Effort estimate: sedang
Diskusikan: [ ] dengan user / [ ] solo
Status: parked
```
