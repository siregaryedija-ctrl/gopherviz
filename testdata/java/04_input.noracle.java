import java.util.Scanner;

// Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak.
public class Main {
    public static void main(String[] args) {
        Scanner sc = new Scanner(System.in);
        System.out.print("Nilai ujian: ");
        int nilai = sc.nextInt();
        String nama = sc.nextLine();
        if (nilai >= 75) {
            System.out.println(nama + " lulus");
        } else {
            System.out.println(nama + " remedial");
        }
        int jumlah = 0;
        while (sc.hasNextInt()) {
            jumlah += sc.nextInt();
        }
        System.out.println("jumlah " + jumlah);
        sc.close();
    }
}
