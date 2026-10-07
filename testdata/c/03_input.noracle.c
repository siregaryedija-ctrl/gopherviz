#include <stdio.h>

/* Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak. */
int main(void) {
    int a, b;
    double x;
    char nama[20];
    printf("Masukkan dua angka: ");
    scanf("%d %d", &a, &b);
    scanf("%lf", &x);
    scanf("%19s", nama);
    if (a > b) {
        printf("%d lebih besar\n", a);
    } else {
        printf("%d lebih besar atau sama\n", b);
    }
    printf("%.1f %s\n", x / 2, nama);
    int c, n = 0;
    while ((c = getchar()) != EOF) {
        n++;
    }
    printf("%d karakter\n", n);
    return 0;
}
