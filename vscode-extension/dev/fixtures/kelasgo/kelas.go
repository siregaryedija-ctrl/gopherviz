package main

import "fmt"

type Akun struct {
	Nama  string
	Saldo int
}

func (a *Akun) Setor(n int) {
	a.Saldo += n
}

func (a Akun) Kosongkan() {
	a.Saldo = 0
}

func main() {
	x := Akun{Nama: "Ani", Saldo: 100}
	y := &Akun{"Budi", 50}
	x.Setor(20)
	y.Setor(5)
	x.Kosongkan()
	fmt.Println(x.Saldo, y.Saldo)
}
