async function load(name) {
  console.log("start " + name);
  await null;
  console.log("resume " + name);
  return name.length;
}

function main() {
  load("A").then(n => console.log("length " + n));
  setTimeout(() => console.log("timer"), 0);
  Promise.resolve().then(() => console.log("promise"));
  console.log("sync done");
}
main();
