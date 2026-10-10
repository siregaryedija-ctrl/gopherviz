import java.util.ArrayDeque;
import java.util.Deque;
import java.util.Queue;

public class Antri {
    static void proses(int[] data) {
        Deque<Integer> tumpukan = new ArrayDeque<>();
        for (int x : data) tumpukan.push(x);
        while (!tumpukan.isEmpty()) System.out.println(tumpukan.pop());
    }
    public static void main(String[] args) {
        proses(new int[]{1, 2, 3});
    }
}
