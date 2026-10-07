// Algoritma pemula: sorting, searching, string, closure, OOP
function bubbleSort(arr) {
  const a = [...arr];
  for (let i = 0; i < a.length - 1; i++) {
    for (let j = 0; j < a.length - 1 - i; j++) {
      if (a[j] > a[j + 1]) {
        [a[j], a[j + 1]] = [a[j + 1], a[j]];
      }
    }
  }
  return a;
}

function binarySearch(arr, target) {
  let lo = 0, hi = arr.length - 1;
  while (lo <= hi) {
    const mid = Math.floor((lo + hi) / 2);
    if (arr[mid] === target) return mid;
    if (arr[mid] < target) lo = mid + 1;
    else hi = mid - 1;
  }
  return -1;
}

const data = [64, 34, 25, 12, 22, 11, 90, 5];
const sorted = bubbleSort(data);
console.log(sorted, data);
console.log("indeks 25:", binarySearch(sorted, 25), "indeks 7:", binarySearch(sorted, 7));

for (let i = 1; i <= 15; i++) {
  let out = "";
  if (i % 3 === 0) out += "Fizz";
  if (i % 5 === 0) out += "Buzz";
  console.log(out || i);
}

const words = "belajar javascript itu menyenangkan".split(" ");
const caps = words.map(w => w[0].toUpperCase() + w.slice(1));
console.log(caps.join(" "), words.filter(w => w.length > 5).length);
const freq = {};
for (const ch of "mississippi") {
  freq[ch] = (freq[ch] || 0) + 1;
}
console.log(freq, Object.entries(freq).sort((a, b) => b[1] - a[1])[0]);

function makeCounter() {
  let count = 0;
  return {
    inc: () => ++count,
    get: () => count,
  };
}
const c1 = makeCounter(), c2 = makeCounter();
c1.inc(); c1.inc(); c2.inc();
console.log(c1.get(), c2.get());

class Animal {
  constructor(name) { this.name = name; }
  speak() { return `${this.name} bersuara`; }
  toString() { return `Animal(${this.name})`; }
}
class Dog extends Animal {
  constructor(name, breed) {
    super(name);
    this.breed = breed;
  }
  speak() { return super.speak() + " guk guk"; }
}
const d = new Dog("Bolt", "husky");
console.log(d.speak(), d instanceof Animal, `${d}`, d);

function grade(score) {
  switch (true) {
    case score >= 90: return "A";
    case score >= 75: return "B";
    default: return "C";
  }
}
console.log([95, 80, 40].map(grade));

let level = 2, msgs = [];
switch (level) {
  case 1:
    msgs.push("satu");
  case 2:
    msgs.push("dua");
  case 3:
    msgs.push("tiga");
    break;
  case 4:
    msgs.push("empat");
}
console.log(msgs);

try {
  const obj = null;
  obj.prop;
} catch (e) {
  console.log("tertangkap:", e.name, e.message);
} finally {
  console.log("finally jalan");
}
function divide(a, b) {
  if (b === 0) throw new RangeError("tidak bisa bagi nol");
  return a / b;
}
try {
  divide(1, 0);
} catch (err) {
  console.log(err instanceof RangeError, err.message);
}

const m = new Map();
m.set("a", 1).set("b", 2);
const s = new Set([3, 1, 3, 2, 1]);
console.log(m.get("b"), m.size, [...s], s.has(2), m);
const json = JSON.stringify({ nama: "Ani", nilai: [90, 85], aktif: true });
console.log(json, JSON.parse(json).nilai[1]);
console.log(JSON.stringify({ a: 1, b: [1, 2] }, null, 2));

console.log((0.1 + 0.2).toFixed(2), (1234.5678).toFixed(1), 10 / 3, 7 % 3, -7 % 3, 2 ** 10);
console.log(parseInt("42px"), parseFloat("3.14abc"), Number("12"), Number("abc"), String(123) + 1);
console.log("5" * "2", "5" + 2, "5" - 2, true + 1, [] + [], null == undefined, null === undefined);
console.log(typeof 42, typeof "x", typeof null, typeof undefined, typeof {}, typeof [], typeof (() => 1));
console.log(Array.from({ length: 12 }, (_, i) => i * 3));
const people = [{ nama: "Budi", umur: 25 }, { nama: "Ani", umur: 22 }, { nama: "Cici", umur: 30 }];
people.sort((a, b) => a.umur - b.umur);
console.log(people.map(p => p.nama), people.reduce((s, p) => s + p.umur, 0));
const { nama, ...lain } = people[0];
console.log(nama, lain, Math.max(...people.map(p => p.umur)));

outer: for (let i = 0; i < 3; i++) {
  for (let j = 0; j < 3; j++) {
    if (j === 2) continue outer;
    if (i === 2) break outer;
    console.log(i, j);
  }
}
let k = 10;
do {
  k -= 3;
} while (k > 0);
console.log("k =", k);
