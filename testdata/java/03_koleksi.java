import java.util.*;

public class Main {
    static class SaldoKurang extends Exception {
        SaldoKurang(String pesan) {
            super(pesan);
        }
    }

    static class Rekening {
        private final String pemilik;
        private long saldo;

        Rekening(String pemilik, long saldo) {
            this.pemilik = pemilik;
            this.saldo = saldo;
        }

        void tarik(long jumlah) throws SaldoKurang {
            if (jumlah > saldo) {
                throw new SaldoKurang("saldo " + pemilik + " kurang " + (jumlah - saldo));
            }
            saldo -= jumlah;
        }

        long getSaldo() {
            return saldo;
        }
    }

    static class Kotak<T> {
        private T isi;

        void simpan(T x) {
            isi = x;
        }

        T ambil() {
            return isi;
        }
    }

    static void bubbleSort(int[] a) {
        int n = a.length;
        for (int i = 0; i < n - 1; i++) {
            boolean tukar = false;
            for (int j = 0; j < n - 1 - i; j++) {
                if (a[j] > a[j + 1]) {
                    int tmp = a[j];
                    a[j] = a[j + 1];
                    a[j + 1] = tmp;
                    tukar = true;
                }
            }
            if (!tukar) {
                break;
            }
        }
    }

    static int faktorial(int n) {
        return n <= 1 ? 1 : n * faktorial(n - 1);
    }

    static double rata(int... nilai) {
        int total = 0;
        for (int v : nilai) {
            total += v;
        }
        return nilai.length == 0 ? 0 : (double) total / nilai.length;
    }

    public static void main(String[] args) {
        int[] data = {64, 25, 12, 22, 11};
        bubbleSort(data);
        System.out.println(Arrays.toString(data));

        Rekening r = new Rekening("Ani", 100_000);
        try {
            r.tarik(30_000);
            r.tarik(80_000);
        } catch (SaldoKurang e) {
            System.out.println("gagal: " + e.getMessage());
        }
        System.out.println("saldo " + r.getSaldo());

        Kotak<String> k = new Kotak<>();
        k.simpan("rahasia");
        System.out.println(k.ambil().toUpperCase());

        Stack<Integer> st = new Stack<>();
        st.push(1);
        st.push(2);
        st.push(3);
        System.out.println(st + " " + st.pop() + " " + st.peek() + " " + st.size());

        Queue<String> antre = new LinkedList<>();
        antre.offer("a");
        antre.offer("b");
        antre.offer("c");
        System.out.println(antre.poll() + " " + antre + " " + antre.peek());

        PriorityQueue<Integer> pq = new PriorityQueue<>(Comparator.reverseOrder());
        for (int x : new int[]{5, 1, 9, 3}) {
            pq.add(x);
        }
        StringBuilder urut = new StringBuilder();
        while (!pq.isEmpty()) {
            urut.append(pq.poll()).append(' ');
        }
        System.out.println(urut.toString().trim());

        Set<Integer> unik = new HashSet<>();
        for (int x : new int[]{3, 17, 3, 1, 17, 40}) {
            unik.add(x);
        }
        System.out.println(unik + " " + unik.contains(40) + " " + unik.size());
        Set<String> abjad = new TreeSet<>(Arrays.asList("kopi", "teh", "air", "susu"));
        System.out.println(abjad);

        List<Integer> angka = new ArrayList<>(List.of(4, 8, 15, 16, 23, 42));
        angka.removeIf(x -> x % 2 == 1);
        angka.forEach(System.out::println);
        System.out.println(faktorial(10) + " " + faktorial(13) + " " + rata(1, 2, 4) + " " + rata());

        Map<Character, Integer> huruf = new TreeMap<>();
        for (char c : "banana".toCharArray()) {
            huruf.merge(c, 1, Integer::sum);
        }
        System.out.println(huruf);

        Integer a = 127, b = 127;
        String s1 = "jav", s2 = s1 + "a";
        System.out.println((a.equals(b)) + " " + s2.equals("java") + " " + "java".compareTo("jawa"));
        long big = Long.MAX_VALUE;
        System.out.println(big + 1);
        System.out.println(10 / 4 * 4 + 10 % 4 + " " + (5 & 3) + " " + (5 | 3) + " " + (5 ^ 3) + " " + (-16 >> 2) + " " + (-16 >>> 28));

        int[] kosong = new int[3];
        String[] kosongS = new String[2];
        boolean[] bs = new boolean[1];
        System.out.println(kosong[1] + " " + kosongS[0] + " " + bs[0]);
        String teks = null;
        System.out.println("akan error");
        System.out.println(teks.length());
    }
}
