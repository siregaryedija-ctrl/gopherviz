// Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak.
const nilai = Number(prompt("Masukkan nilai:"));
if (nilai >= 80) {
  console.log("A");
} else if (nilai >= 60) {
  console.log("B");
} else {
  console.log("C");
}
