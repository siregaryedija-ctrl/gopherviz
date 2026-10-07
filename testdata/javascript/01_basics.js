// Dasar: loop, if, switch, fungsi
function fib(n) {
  if (n < 2) {
    return n;
  }
  return fib(n - 1) + fib(n - 2);
}

const nums = [5, 3, 8, 1];
let total = 0;
for (let i = 0; i < nums.length; i++) {
  total += nums[i];
}
console.log("total", total);

for (const x of nums) {
  if (x > 4) {
    console.log(x, "besar");
  } else if (x > 2) {
    console.log(x, "sedang");
  } else {
    console.log(x, "kecil");
  }
}

let k = 3;
while (k > 0) {
  k--;
}

do {
  k++;
} while (k < 2);

switch (k) {
  case 1:
    console.log("satu");
    break;
  case 2:
    console.log("dua");
    break;
  default:
    console.log("lain");
}

const person = { name: "Ani", age: 20 };
for (const key in person) {
  console.log(`${key} = ${person[key]}`);
}
console.log(fib(6));
