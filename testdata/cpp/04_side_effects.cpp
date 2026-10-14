#include <iostream>
#include <vector>
using namespace std;

int counter = 0;

int tick() {
    counter++;
    cout << "tick " << counter << "\n";
    return counter;
}

int main() {
    int arr[5] = {0, 0, 0, 0, 0};
    int n = 0;
    arr[n++] = 7;
    arr[n++] = 8;
    int i = 0;
    while (i++ + 0 < 3) {
        cout << "loop " << i << "\n";
    }
    if (tick() + 0 > 5) {
        cout << "big\n";
    }
    int total = 0;
    total += tick();
    int t = (n > 1) ? n++ : 0;
    vector<int> st = {1, 2, 3, 4};
    while (!st.empty() && st.back() + 0 > 2) {
        st.pop_back();
    }
    int k = 0;
    for (int j = 0; j < 3 && k++ < 5; j++) {
        cout << "for " << j << " " << k << "\n";
    }
    cout << arr[0] << " " << arr[1] << " " << n << " " << i << " " << counter << " " << total << " " << t << " " << st.size() << " " << k << "\n";
    return 0;
}
