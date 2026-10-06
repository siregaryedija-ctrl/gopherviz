# GoLoopViz

Lihat cara kerja loop Go sebagai level game pixel-art, langsung saat kamu mengetik.

- **Realtime.** Ketik `for i := 0; i < 3; i++ {}` dan level-nya langsung muncul. Body kosong tampil sebagai blok bata
  kosong; setiap baris yang kamu tulis di dalam `{ }` menjadi blok baru.
- **Mengikuti kursor.** Panel selalu menampilkan loop tempat kursormu berada. Klik bagian di panel untuk melompat ke barisnya.
- **Gopher menelusuri alurnya.** Init, gerbang kondisi (T/F), menyundul blok satu per satu, post, lalu kembali ke gerbang.
  `break` digambar sebagai pipa, `continue` sebagai pegas, `return` sebagai pintu, loop di dalam loop sebagai pipa ungu.
- **Kode setengah jadi tidak apa-apa.** Loop yang belum lengkap tampil sebagai blok "hantu" sampai selesai diketik.
- Efek suara dan musik chiptune 8-bit (bisa dimatikan).

Saat ini panel menampilkan **mode struktur**: alur loop dengan contoh 3 putaran. Nilai variabel asli di setiap putaran
akan hadir di versi berikutnya.

## Pemakaian

1. Buka file `.go`. Ekstensi aktif otomatis.
2. Taruh kursor di dalam sebuah `for`. Panel GoLoopViz terbuka di samping (bisa dimatikan lewat `goloopviz.autoOpen`).
3. Atau klik **▶ Visualisasikan loop** di atas `for`, atau tekan `Ctrl+Alt+L`.

## Pengaturan

| Setting | Bawaan | Fungsi |
|---|---|---|
| `goloopviz.realtime` | `true` | Perbarui di setiap ketikan; `false` = hanya saat disimpan |
| `goloopviz.autoOpen` | `true` | Buka panel otomatis saat kursor masuk ke loop |
| `goloopviz.codeLens` | `true` | Tombol "Visualisasikan loop" di atas `for` |
| `goloopviz.speed` | `1` | Kecepatan animasi: 0.5, 1, 2, 4 |
| `goloopviz.sound` / `goloopviz.music` | `true` | Efek suara / musik latar |
| `goloopviz.binaryPath` | kosong | Pakai binary `loopviz` sendiri, bukan bawaan ekstensi |

## Cara kerja

Ekstensi menjalankan `loopviz serve` (ditulis dalam Go, ikut di dalam paket ekstensi) dan mengirim isi editor setiap kamu
mengetik. `loopviz` membaca kode dengan parser Go asli lalu mengirim balik struktur loop-nya. Kode kamu tidak pernah dijalankan.

## Atribusi

Gopher pixel digambar ulang dari desain Go Gopher karya Renée French, lisensi Creative Commons Attribution 4.0.
Semua suara dibangkitkan langsung dengan Web Audio, tanpa file audio.
