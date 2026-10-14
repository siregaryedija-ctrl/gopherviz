let counter = 0;
function tick() {
  counter++;
  console.log("tick " + counter);
  return counter;
}
function main() {
  const arr = [0, 0, 0, 0, 0];
  let n = 0;
  arr[n++] = 7;
  arr[n++] = 8;
  let i = 0;
  while (i++ + 0 < 3) {
    console.log("loop " + i);
  }
  if (tick() + 0 > 5) {
    console.log("big");
  }
  let total = 0;
  total += tick();
  const t = n > 1 ? n++ : 0;
  const st = [1, 2, 3, 4];
  while (st.length && st.pop() > 2) {
    console.log("popped");
  }
  let k = 0;
  for (let j = 0; j < 3 && k++ < 5; j++) {
    console.log("for " + j + " " + k);
  }
  const q = [5, 6, 7];
  while (q.shift() < 7) {
    console.log("shift " + q.length);
  }
  console.log(arr[0], arr[1], n, i, counter, total, t, st.length, k, q.length);
}
main();
