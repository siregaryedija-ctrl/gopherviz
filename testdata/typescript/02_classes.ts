// OOP TypeScript: parameter properties, abstract, generic, enum string
enum Direction {
  Up = "UP",
  Down = "DOWN",
}

interface Shape {
  area(): number;
  name: string;
}

abstract class Base implements Shape {
  constructor(public name: string, protected readonly id: number) {}
  abstract area(): number;
  describe(): string {
    return `${this.name}#${this.id} luas ${this.area().toFixed(2)}`;
  }
}

class Circle extends Base {
  constructor(private r: number) {
    super("lingkaran", 1);
  }
  area(): number {
    return Math.PI * this.r ** 2;
  }
}

class Square extends Base {
  constructor(private side: number) {
    super("persegi", 2);
  }
  area(): number {
    return this.side * this.side;
  }
}

function largest<T extends Shape>(items: T[]): T | undefined {
  let best: T | undefined = undefined;
  for (const item of items) {
    if (best === undefined || item.area() > best.area()) {
      best = item;
    }
  }
  return best;
}

const shapes: Base[] = [new Circle(2), new Square(3), new Circle(1)];
for (const s of shapes) {
  console.log(s.describe());
}
console.log(largest(shapes)?.name, Direction.Up, Direction["Down"]);

type Pair<K, V> = { key: K; value: V };
const pairs: Pair<string, number>[] = [
  { key: "a", value: 3 },
  { key: "b", value: 1 },
];
const total = pairs.reduce((acc: number, p) => acc + p.value, 0);
const byKey = new Map<string, number>(pairs.map((p): [string, number] => [p.key, p.value]));
console.log(total, byKey.get("a"), byKey);

function describe(x: number | string | boolean): string {
  if (typeof x === "number") {
    return x > 0 ? "positif" : "bukan positif";
  } else if (typeof x === "string") {
    return x.toUpperCase();
  }
  return x ? "ya" : "tidak";
}
console.log(describe(5), describe("hai"), describe(false));
let maybe: string | null = null;
console.log(maybe ?? "default", (maybe as string | null)?.length);
const tuple: [string, number] = ["umur", 21];
const [label, value] = tuple;
console.log(`${label}: ${value}`);
