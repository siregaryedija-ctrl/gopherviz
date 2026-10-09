import { luas, keliling } from './rumus.js';
import rupiah from './format.js';

function hitung(p, l) {
  const a = luas(p, l);
  const k = keliling(p, l);
  console.log(rupiah(a), k);
  return a + k;
}
hitung(3, 4);
