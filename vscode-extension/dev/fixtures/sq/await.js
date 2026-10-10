async function ambil(nama) {
  console.log("mulai " + nama);
  await null;
  console.log("lanjut " + nama);
  return nama.length;
}

function main() {
  ambil("A").then(n => console.log("panjang " + n));
  setTimeout(() => console.log("timer"), 0);
  Promise.resolve().then(() => console.log("promise"));
  console.log("sinkron selesai");
}
main();
