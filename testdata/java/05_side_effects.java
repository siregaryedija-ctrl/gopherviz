import java.util.ArrayList;
import java.util.ArrayDeque;
import java.util.List;

public class Main {
    static int counter = 0;

    static int tick() {
        counter++;
        System.out.println("tick " + counter);
        return counter;
    }

    public static void main(String[] args) {
        int[] arr = new int[5];
        int n = 0;
        arr[n++] = 7;
        arr[n++] = 8;
        int i = 0;
        while (i++ + 0 < 3) {
            System.out.println("loop " + i);
        }
        if (tick() + 0 > 5) {
            System.out.println("big");
        }
        int total = 0;
        total += tick();
        int t = n > 1 ? n++ : 0;
        List<Integer> st = new ArrayList<>(List.of(1, 2, 3, 4));
        while (!st.isEmpty() && st.remove(st.size() - 1) > 2) {
            System.out.println("popped");
        }
        int k = 0;
        for (int j = 0; j < 3 && k++ < 5; j++) {
            System.out.println("for " + j + " " + k);
        }
        ArrayDeque<Integer> q = new ArrayDeque<>(List.of(5, 6, 7));
        while (q.poll() < 7) {
            System.out.println("shift " + q.size());
        }
        System.out.println(arr[0] + " " + arr[1] + " " + n + " " + i + " " + counter + " " + total + " " + t + " " + st.size() + " " + k + " " + q.size());
    }
}
