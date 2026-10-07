class Counter {
  #count = 0;
  static created = 0;

  constructor(start) {
    this.#count = start;
    Counter.created++;
  }

  inc(by = 1) {
    this.#count += by;
    return this;
  }

  get value() {
    return this.#count;
  }
}

const square = x => x * x;
const add = (a, b) => {
  return a + b;
};
const doubled = [1, 2, 3].map(n => n * 2);
const [first, ...rest] = doubled;
const { a, b: renamed = 5 } = { a: 1 };
const re = /ab+c/i;
const c = new Counter(1).inc().inc(3);
console.log(c.value, square(4), add(1, 2), first, rest, a, renamed, re.test("abbc"));
const msg = c.value > 3 ? "besar" : "kecil"
console.log(msg ?? "kosong", c?.value)

outer: for (let i = 0; i < 3; i++) {
  for (let j = 0; j < 3; j++) {
    if (j === 1) continue outer;
    if (i === 2) break outer;
  }
}

async function load() {
  try {
    const r = await Promise.resolve(42);
    return r;
  } catch (e) {
    console.error(e.message);
  } finally {
    console.log("selesai");
  }
}
load().then(v => console.log(v));
