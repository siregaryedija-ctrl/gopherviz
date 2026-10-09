class Akun {
  constructor(nama, saldo) {
    this.nama = nama;
    this.saldo = saldo;
  }
  setor(n) {
    this.saldo += n;
    return this.saldo;
  }
}
class Tabungan extends Akun {
  constructor(nama, saldo, bunga) {
    super(nama, saldo);
    this.bunga = bunga;
  }
  tambahBunga() {
    this.setor(this.saldo * this.bunga / 100);
  }
}
const a = new Akun("Ani", 100);
const t = new Tabungan("Budi", 200, 10);
a.setor(50);
t.tambahBunga();
console.log(a.saldo, t.saldo);
