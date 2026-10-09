package main

import "fmt"

func main() {
	a := &Akun{Nama: "Ani", Saldo: 100}
	for i := 0; i < 2; i++ {
		a.Setor(Bunga(a.Saldo))
	}
	fmt.Println(a.Saldo)
}
