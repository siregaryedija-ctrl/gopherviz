// C lanjut: struct, pointer, string, memori dinamis, rekursi
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

#define MAX 5

typedef struct {
    char nama[20];
    int nilai;
} Mahasiswa;

typedef enum { MERAH, KUNING = 5, HIJAU } Warna;

void tukar(int *a, int *b) {
    int t = *a;
    *a = *b;
    *b = t;
}

void urutkan(int arr[], int n) {
    for (int i = 0; i < n - 1; i++) {
        for (int j = 0; j < n - 1 - i; j++) {
            if (arr[j] > arr[j + 1]) {
                tukar(&arr[j], &arr[j + 1]);
            }
        }
    }
}

long faktorial(int n) {
    return n <= 1 ? 1 : n * faktorial(n - 1);
}

int fib(int n) {
    static int memo[50];
    if (n < 2) return n;
    if (memo[n]) return memo[n];
    return memo[n] = fib(n - 1) + fib(n - 2);
}

void kapital(char *s) {
    for (int i = 0; s[i] != '\0'; i++) {
        s[i] = toupper(s[i]);
    }
}

int hitung(void) {
    static int n = 0;
    n++;
    return n;
}

int main(void) {
    int data[MAX] = {42, 7, 19, 3, 25};
    urutkan(data, MAX);
    for (int i = 0; i < MAX; i++) {
        printf("%d%s", data[i], i < MAX - 1 ? ", " : "\n");
    }

    Mahasiswa kelas[3] = {{"Ani", 85}, {"Budi", 72}, {"Cici", 91}};
    Mahasiswa *terbaik = &kelas[0];
    for (int i = 1; i < 3; i++) {
        if (kelas[i].nilai > terbaik->nilai) {
            terbaik = &kelas[i];
        }
    }
    printf("terbaik: %s (%d)\n", terbaik->nama, terbaik->nilai);
    strcpy(kelas[1].nama, "Bambang");
    printf("%s %zu %d\n", kelas[1].nama, strlen(kelas[1].nama), strcmp("abc", "abd") < 0);

    char kata[30] = "halo dunia";
    kapital(kata);
    strcat(kata, "!");
    printf("%s (%d huruf)\n", kata, (int)strlen(kata));

    int n = 6;
    int *dinamis = malloc(n * sizeof(int));
    for (int i = 0; i < n; i++) {
        dinamis[i] = i * i;
    }
    int *p = dinamis;
    int total = 0;
    while (p < dinamis + n) {
        total += *p;
        p++;
    }
    printf("total kuadrat %d, elemen ke-3 %d\n", total, *(dinamis + 3));
    free(dinamis);

    printf("%ld %d %d\n", faktorial(10), fib(30), hitung() + hitung() + hitung());
    Warna w = HIJAU;
    switch (w) {
        case MERAH:
            printf("merah\n");
        case KUNING:
            printf("kuning\n");
        case HIJAU:
            printf("hijau %d\n", w);
        default:
            printf("selesai\n");
    }
    unsigned int flags = 0;
    flags |= 1 << 2;
    flags |= 1 << 0;
    printf("flags=%u bit2=%d neg=%d hex=%x oct=%o\n", flags, (flags >> 2) & 1, ~0, 255, 8);
    int matriks[2][3] = {{1, 2, 3}, {4, 5, 6}};
    int jumlah = 0;
    for (int i = 0; i < 2; i++)
        for (int j = 0; j < 3; j++)
            jumlah += matriks[i][j];
    double rata = jumlah / 6.0;
    printf("jumlah %d rata %.3f %5.1f|%-6s|%06.2f\n", jumlah, rata, 3.14159, "ok", 3.14159);
    int x = 10;
    do {
        x -= 3;
    } while (x > 0);
    char c = 'a';
    printf("x=%d c=%c next=%c ascii=%d %d %d\n", x, c, c + 1, c, 7 / 2, -7 % 3);
    return 0;
}
