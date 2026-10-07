import java.util.ArrayList;
import java.util.List;

public class Main {
    static int fib(int n) {
        if (n < 2) {
            return n;
        }
        return fib(n - 1) + fib(n - 2);
    }

    public static void main(String[] args) {
        int[] nums = {5, 3, 8, 1};
        int total = 0;
        for (int i = 0; i < nums.length; i++) {
            total += nums[i];
        }
        List<String> names = new ArrayList<>();
        names.add("Ani");
        names.add("Budi");
        for (String name : names) {
            if (name.startsWith("A")) {
                System.out.println(name + " diawali A");
            } else {
                System.out.println(name);
            }
        }
        int k = 3;
        while (k > 0) {
            k--;
        }
        do {
            k++;
        } while (k < 2);
        switch (k) {
            case 1:
                System.out.println("satu");
                break;
            case 2:
                System.out.println("dua");
                break;
            default:
                System.out.println("lain");
        }
        Point p = new Point(1, 2);
        int[] grid = new int[3];
        double avg = (double) total / nums.length;
        System.out.println(total + " " + fib(6) + " " + p.sum() + " " + grid.length + " " + avg);
    }
}

class Point {
    private int x, y;

    Point(int x, int y) {
        this.x = x;
        this.y = y;
    }

    int sum() {
        return x + y;
    }
}
