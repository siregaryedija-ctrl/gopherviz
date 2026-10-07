#include <iostream>
#include <vector>
#include <string>
using namespace std;

struct Point {
    int x;
    int y;
};

class Rect {
public:
    Rect(int w, int h) : w(w), h(h) {}
    int area() const { return w * h; }
private:
    int w, h;
};

int fib(int n) {
    if (n < 2) return n;
    return fib(n - 1) + fib(n - 2);
}

int main() {
    vector<int> v = {5, 3, 8, 1};
    int total = 0;
    for (int i = 0; i < (int)v.size(); i++) {
        total += v[i];
    }
    for (int x : v) {
        if (x > 4) {
            cout << x << " besar" << endl;
        } else {
            cout << x << " kecil" << endl;
        }
    }
    int arr[3] = {1, 2, 3};
    Point p = {1, 2};
    Rect r(3, 4);
    int k = 0;
    while (k < 3) k++;
    do {
        k--;
    } while (k > 0);
    switch (k) {
        case 0:
            cout << "nol" << endl;
            break;
        default:
            cout << "lain" << endl;
    }
    string s = "halo";
    auto sq = [](int a) { return a * a; };
    cout << total << " " << fib(6) << " " << r.area() << " " << p.x + arr[2] << " " << s << " " << sq(3) << endl;
    return 0;
}
