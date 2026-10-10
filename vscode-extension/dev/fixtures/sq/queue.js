function layani(pelanggan) {
  const antrian = [];
  for (const p of pelanggan) antrian.push(p);
  while (antrian.length > 0) {
    const orang = antrian.shift();
    console.log("melayani", orang);
  }
}
layani(["Ani", "Budi", "Cici"]);
