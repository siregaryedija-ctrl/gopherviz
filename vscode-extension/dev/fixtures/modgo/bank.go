package main

type Akun struct {
	Nama  string
	Saldo int
}

func (a *Akun) Setor(n int) {
	a.Saldo += n
}

func Bunga(saldo int) int {
	return saldo / 10
}
