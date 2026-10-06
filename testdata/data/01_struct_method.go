package main

import "fmt"

type Person struct {
	Name string
	Age  int
}

func (p Person) Greet() string {
	return "Halo, aku " + p.Name
}

func (p *Person) Birthday() {
	p.Age++
}

func main() {
	ani := Person{Name: "Ani", Age: 20}
	ani.Birthday()
	fmt.Println(ani.Greet(), ani.Age)

	people := []Person{{"Budi", 30}, {"Citra", 25}}
	for _, p := range people {
		p.Age = 99 // hanya mengubah salinan
	}
	for i := range people {
		people[i].Birthday()
	}
	fmt.Println(people)
	fmt.Printf("%+v\n", people[0])

	copyAni := ani
	copyAni.Name = "Salinan"
	fmt.Println(ani.Name, copyAni.Name)
}
