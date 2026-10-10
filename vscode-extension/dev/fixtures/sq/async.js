function main() {
  console.log("1 mulai");
  setTimeout(() => console.log("4 timeout"), 0);
  Promise.resolve().then(() => console.log("3 promise"));
  console.log("2 selesai");
}
main();
