#include <stdio.h>

int counter = 0;

int tick(void) {
    counter++;
    printf("tick %d\n", counter);
    return counter;
}

int main(void) {
    int arr[5] = {0, 0, 0, 0, 0};
    int n = 0;
    arr[n++] = 7;
    arr[n++] = 8;
    int i = 0;
    while (i++ + 0 < 3) {
        printf("loop %d\n", i);
    }
    if (tick() + 0 > 5) {
        printf("big\n");
    }
    int total = 0;
    total += tick();
    int t = (n > 1) ? n++ : 0;
    int k = 0;
    for (int j = 0; j < 3 && k++ < 5; j++) {
        printf("for %d %d\n", j, k);
    }
    int m = 10;
    do {
        m -= 3;
    } while (--m > 0);
    printf("%d %d %d %d %d %d %d %d %d\n", arr[0], arr[1], n, i, counter, total, t, k, m);
    return 0;
}
