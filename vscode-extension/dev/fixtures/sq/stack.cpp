#include <iostream>
#include <queue>

void proses() {
    std::queue<int> q;
    for (int i = 1; i <= 3; i++) q.push(i * 10);
    while (!q.empty()) {
        std::cout << q.front() << "\n";
        q.pop();
    }
}

int main() { proses(); }
