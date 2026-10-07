import java.util.*;

public class Main {
    enum Warna { MERAH, HIJAU, BIRU }

    enum Level {
        RENDAH(1), TINGGI(3);

        final int bobot;

        Level(int bobot) {
            this.bobot = bobot;
        }
    }

    interface Bentuk {
        double luas();

        String nama();

        default String info() {
            return nama() + " luas " + String.format("%.2f", luas());
        }
    }

    record Titik(int x, int y) {
        int jarak() {
            return Math.abs(x) + Math.abs(y);
        }
    }

    static abstract class Hewan {
        protected String nama;

        Hewan(String nama) {
            this.nama = nama;
        }

        abstract String suara();

        public String toString() {
            return nama + " bilang " + suara();
        }
    }

    static class Kucing extends Hewan {
        Kucing(String n) {
            super(n);
        }

        String suara() {
            return "meong";
        }
    }

    static class Anjing extends Hewan {
        Anjing(String n) {
            super(n);
        }

        String suara() {
            return "guk";
        }
    }

    static class Persegi implements Bentuk {
        private final double sisi;

        Persegi(double s) {
            sisi = s;
        }

        public double luas() {
            return sisi * sisi;
        }

        public String nama() {
            return "Persegi";
        }
    }

    static class Lingkaran implements Bentuk {
        private final double r;

        Lingkaran(double r) {
            this.r = r;
        }

        public double luas() {
            return Math.PI * r * r;
        }

        public String nama() {
            return "Lingkaran";
        }
    }

    static int counter = 0;

    static int bagi(int a, int b) {
        counter++;
        return a / b;
    }

    public static void main(String[] args) {
        for (Warna w : Warna.values()) {
            switch (w) {
                case MERAH:
                    System.out.println(w + " " + w.ordinal());
                    break;
                case HIJAU:
                    System.out.println(w.name().toLowerCase());
                    break;
                default:
                    System.out.println("lainnya: " + w);
            }
        }
        System.out.println(Level.TINGGI.bobot * 2 + " " + Level.valueOf("RENDAH"));

        Titik t = new Titik(3, -4);
        Titik u = new Titik(3, -4);
        System.out.println(t + " " + t.x() + " " + t.jarak() + " " + t.equals(u));

        List<Hewan> hewan = new ArrayList<>();
        hewan.add(new Kucing("Tom"));
        hewan.add(new Anjing("Rex"));
        for (Hewan h : hewan) {
            System.out.println(h);
        }

        Bentuk[] bentuk = {new Persegi(2.5), new Lingkaran(1)};
        for (Bentuk b : bentuk) {
            System.out.println(b.info());
        }

        try {
            bagi(10, 0);
        } catch (ArithmeticException e) {
            System.out.println("error: " + e.getMessage());
        } finally {
            System.out.println("counter " + counter);
        }
        try {
            int[] arr = new int[2];
            arr[5] = 1;
        } catch (ArrayIndexOutOfBoundsException e) {
            System.out.println(e.getMessage());
        }
        try {
            Integer.parseInt("12a");
        } catch (NumberFormatException e) {
            System.out.println(e);
        }

        Map<String, Integer> stok = new HashMap<>();
        String[] buah = {"apel", "jeruk", "mangga", "apel", "pisang", "jeruk", "apel"};
        for (String b : buah) {
            stok.put(b, stok.getOrDefault(b, 0) + 1);
        }
        System.out.println(stok);
        for (Map.Entry<String, Integer> e : stok.entrySet()) {
            if (e.getValue() > 1) {
                System.out.println(e.getKey() + "=" + e.getValue());
            }
        }
        TreeMap<String, Integer> urut = new TreeMap<>(stok);
        System.out.println(urut);

        StringBuilder sb = new StringBuilder();
        for (char c = 'a'; c <= 'e'; c++) {
            sb.append(c);
        }
        sb.reverse();
        System.out.println(sb.toString() + " " + sb.length());
        char c = 'A';
        c += 2;
        System.out.println(c + " " + (c + 1) + " " + (char) (c + 1));
        String kata = "Halo Dunia";
        int vokal = 0;
        for (char ch : kata.toLowerCase().toCharArray()) {
            if ("aiueo".indexOf(ch) >= 0) {
                vokal++;
            }
        }
        System.out.println("vokal " + vokal);

        int besar = Integer.MAX_VALUE;
        besar++;
        long l = 1L << 40;
        System.out.println(besar + " " + l + " " + (7 / 2) + " " + (-7 / 2) + " " + (-7 % 3) + " " + 7.0 / 2);
        System.out.println(0.1 + 0.2);
        System.out.println(100.0 / 3 + " " + (float) 1.1 + " " + 1e10 + " " + 1.0E-5 + " " + 123456789.0);

        List<String> nama = new ArrayList<>(Arrays.asList("Citra", "ani", "Budi", "dodi"));
        Collections.sort(nama);
        System.out.println(nama);
        nama.sort((a, b) -> a.length() - b.length());
        System.out.println(nama);
        nama.forEach(n -> System.out.print(n.charAt(0)));
        System.out.println();

        System.out.println(String.format("%-6s|%5d|%08.3f|%x", "ab", 42, 3.14159, 255));
        System.out.printf("%s punya %d item, total %,d%n", "Ani", 3, 1234567);
        String s = "  Belajar Java  ";
        System.out.println("[" + s.trim() + "] " + s.trim().substring(8) + " " + s.contains("Java") + " " + String.join("-", "a", "b", "c"));
        String[] parts = "a,b,,c,,".split(",");
        System.out.println(parts.length + " " + Arrays.toString(parts));

        String hari = "Sabtu";
        switch (hari) {
            case "Sabtu":
            case "Minggu":
                System.out.println("libur");
                break;
            default:
                System.out.println("kerja");
        }
        int nilai = 75;
        String grade = nilai >= 80 ? "A" : nilai >= 70 ? "B" : "C";
        System.out.println("grade " + grade);

        int[][] m = new int[2][3];
        m[1][2] = 5;
        System.out.println(Arrays.deepToString(m));
        outer:
        for (int i = 1; i <= 4; i++) {
            for (int j = 1; j <= 4; j++) {
                if (i * j == 6) {
                    System.out.println(i + "," + j);
                    break outer;
                }
            }
        }
    }
}
