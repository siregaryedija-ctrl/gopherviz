#include <stdio.h>
#define N 5

typedef struct {
    int x;
    int y;
} Point;

int sum(int arr[], int n) {
    int total = 0;
    for (int i = 0; i < n; i++) {
        total += arr[i];
    }
    return total;
}

int main(void) {
    int data[N] = {4, 2, 9, 1, 5};
    int i = 0;
    while (i < N) {
        if (data[i] % 2 == 0) {
            printf("%d genap\n", data[i]);
        } else {
            printf("%d ganjil\n", data[i]);
        }
        i++;
    }
    Point p = {3, 4};
    Point *pp = &p;
    pp->x = 10;
    char c = 'A';
    switch (c) {
        case 'A':
            printf("huruf A\n");
            break;
        default:
            printf("lain\n");
    }
    double avg = (double)sum(data, N) / N;
    printf("rata-rata %.2f, x=%d\n", avg, p.x);
    return 0;
}
