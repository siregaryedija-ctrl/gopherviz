// Event loop: urutan output async (dibandingkan dengan Node asli oleh TestAgainstReal).
function tunggu(ms, nilai) {
  return new Promise(resolve => setTimeout(() => resolve(nilai), ms));
}

async function ambil(nama) {
  console.log("mulai " + nama);
  await null;
  console.log("lanjut " + nama);
  return nama.length;
}

async function berurutan() {
  const a = await tunggu(20, "a");
  const b = await tunggu(10, "b");
  console.log("berurutan", a + b);
  try {
    await Promise.reject(new Error("gagal"));
  } catch (e) {
    console.log("tertangkap", e.message);
  }
  return "selesai";
}

async function bertingkat() {
  const n = await ambil("dalam");
  console.log("bertingkat dapat", n);
}

console.log("1 sinkron");
ambil("A").then(n => console.log("panjang", n));
setTimeout(() => console.log("timer 0"), 0);
setTimeout(() => console.log("timer 5"), 5);
Promise.resolve(1).then(x => x + 1).then(x => console.log("rantai", x));
queueMicrotask(() => console.log("microtask"));
berurutan().then(s => console.log(s));
bertingkat();
console.log("2 sinkron");
