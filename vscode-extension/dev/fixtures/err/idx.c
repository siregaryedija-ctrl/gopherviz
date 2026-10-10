#include <stdio.h>

int jumlah(int a[], int n) {
    int s = 0;
    for (int i = 0; i <= n; i++) {
        s += a[i];
    }
    return s;
}

int main(void) {
    int a[3] = {5, 1, 4};
    printf("%d\n", jumlah(a, 3));
    return 0;
}
