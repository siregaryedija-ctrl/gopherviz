interface Point {
  x: number;
  y: number;
}

type Shape = "circle" | "square";

enum Color {
  Red,
  Green = 5,
  Blue,
}

function area(shape: Shape, size: number): number {
  switch (shape) {
    case "circle":
      return Math.PI * size * size;
    case "square":
      return size * size;
  }
}

class Stack<T> {
  private items: T[] = [];

  push(item: T): void {
    this.items.push(item);
  }

  pop(): T | undefined {
    return this.items.pop();
  }

  get size(): number {
    return this.items.length;
  }
}

const s = new Stack<number>();
for (let i: number = 0; i < 3; i++) {
  s.push(i * 10);
}
while (s.size > 0) {
  const v = s.pop()!;
  console.log(v);
}
const p: Point = { x: 1, y: 2 };
const dist = (a: Point, b: Point): number => Math.hypot(a.x - b.x, a.y - b.y);
console.log(area("square", 3), dist(p, { x: 4, y: 6 }), Color.Blue as number);
