#include <iostream>
#include <string>
using namespace std;

// Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak.
int main() {
    int umur;
    string nama;
    cout << "Umur: ";
    cin >> umur;
    cin.ignore();
    getline(cin, nama);
    if (umur >= 17) {
        cout << nama << " boleh punya KTP" << endl;
    } else {
        cout << nama << " belum boleh" << endl;
    }
    int x, total = 0, banyak = 0;
    while (cin >> x) {
        total += x;
        banyak++;
    }
    cout << banyak << " angka, total " << total << endl;
    return 0;
}
