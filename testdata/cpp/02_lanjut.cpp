// C++ lanjut: OOP, template, STL, pointer, exception
#include <iostream>
#include <iomanip>
#include <vector>
#include <map>
#include <set>
#include <stack>
#include <queue>
#include <string>
#include <algorithm>
#include <numeric>
#include <stdexcept>
using namespace std;

class Shape {
public:
    Shape(const string& name) : name(name) { count++; }
    virtual ~Shape() {}
    virtual double area() const = 0;
    string describe() const {
        return name + " luas " + to_string(area());
    }
    static int count;
protected:
    string name;
};

int Shape::count = 0;

class Circle : public Shape {
public:
    Circle(double r) : Shape("lingkaran"), r(r) {}
    double area() const override { return 3.14159 * r * r; }
private:
    double r;
};

class Rect : public Shape {
public:
    Rect(double w, double h) : Shape("persegi panjang"), w(w), h(h) {}
    double area() const override { return w * h; }
private:
    double w, h;
};

struct Point {
    int x, y;
    Point operator+(const Point& o) const { return {x + o.x, y + o.y}; }
    bool operator<(const Point& o) const { return x < o.x || (x == o.x && y < o.y); }
};

ostream& operator<<(ostream& os, const Point& p) {
    os << "(" << p.x << ", " << p.y << ")";
    return os;
}

template <typename T>
T maxOf(T a, T b) {
    return a > b ? a : b;
}

template <typename T>
class Box {
public:
    Box(T v) : value(v) {}
    T get() const { return value; }
private:
    T value;
};

struct Node {
    int data;
    Node* next;
    Node(int d) : data(d), next(nullptr) {}
};

class InsufficientFunds : public runtime_error {
public:
    InsufficientFunds(int need) : runtime_error("saldo kurang " + to_string(need)) {}
};

void withdraw(int& balance, int amount) {
    if (amount > balance) throw InsufficientFunds(amount - balance);
    balance -= amount;
}

int counter() {
    static int calls = 0;
    return ++calls;
}

int main() {
    vector<Shape*> shapes = {new Circle(1.5), new Rect(2, 3)};
    for (Shape* s : shapes) {
        cout << s->describe() << endl;
    }
    cout << "jumlah bentuk: " << Shape::count << endl;

    Point a{1, 2}, b{3, 4};
    Point c = a + b;
    cout << c << " " << (a < b) << endl;
    vector<Point> pts = {{3, 1}, {1, 5}, {1, 2}};
    sort(pts.begin(), pts.end());
    for (const auto& p : pts) cout << p << " ";
    cout << endl;

    cout << maxOf(3, 7) << " " << maxOf(2.5, 1.5) << " " << maxOf<string>("apel", "jeruk") << endl;
    Box<int> bi(42);
    Box<string> bs("halo");
    cout << bi.get() << " " << bs.get() << endl;

    vector<int> nums = {5, 2, 8, 1, 9, 3};
    sort(nums.begin(), nums.end(), [](int x, int y) { return x > y; });
    cout << "urut turun:";
    for (int n : nums) cout << " " << n;
    cout << endl;
    int total = accumulate(nums.begin(), nums.end(), 0);
    cout << "total " << total << " max " << *max_element(nums.begin(), nums.end()) << " ada 8? "
         << (find(nums.begin(), nums.end(), 8) != nums.end()) << endl;
    reverse(nums.begin(), nums.end());
    nums.push_back(10);
    nums.erase(nums.begin());
    cout << nums.size() << " " << nums.front() << " " << nums.back() << endl;

    map<string, int> freq;
    string words[] = {"apel", "jeruk", "apel", "mangga", "apel", "jeruk"};
    for (const string& w : words) freq[w]++;
    for (const auto& [word, n] : freq) cout << word << ":" << n << " ";
    cout << freq.count("pisang") << endl;
    set<int> uniq = {3, 1, 3, 2, 1};
    cout << uniq.size() << " " << *uniq.begin() << endl;

    stack<int> st;
    queue<string> q;
    for (int i = 1; i <= 3; i++) {
        st.push(i * 10);
        q.push("q" + to_string(i));
    }
    cout << st.top() << " " << q.front() << " ";
    st.pop();
    q.pop();
    cout << st.top() << " " << q.front() << " " << st.size() << endl;
    priority_queue<int> pq;
    for (int x : {4, 9, 2}) pq.push(x);
    cout << "pq top " << pq.top() << endl;

    Node* head = new Node(1);
    head->next = new Node(2);
    head->next->next = new Node(3);
    int sum = 0;
    for (Node* cur = head; cur != nullptr; cur = cur->next) sum += cur->data;
    cout << "linked list sum " << sum << endl;

    int balance = 100;
    try {
        withdraw(balance, 30);
        withdraw(balance, 200);
    } catch (const InsufficientFunds& e) {
        cout << "error: " << e.what() << endl;
    }
    cout << "saldo " << balance << endl;
    try {
        vector<int> small = {1, 2};
        cout << small.at(5) << endl;
    } catch (const out_of_range& e) {
        cout << "out_of_range tertangkap" << endl;
    }

    string s = "Belajar C++";
    s += " itu seru";
    cout << s.size() << " " << s.substr(8, 3) << " " << s.find("seru") << " " << (s.find("xyz") == string::npos) << endl;
    string rev = s;
    for (size_t i = 0, j = rev.size() - 1; i < j; i++, j--) swap(rev[i], rev[j]);
    cout << rev << endl;

    counter();
    counter();
    cout << "counter " << counter() << endl;
    double avg = static_cast<double>(total) / nums.size();
    cout << fixed << setprecision(2) << avg << " " << setw(8) << 3.14159 << "|" << endl;
    char grade = 'A';
    grade += 2;
    cout << grade << " " << int(grade) << " " << (7 / 2) << " " << (7 % 3) << " " << (-7 / 2) << endl;
    for (Shape* s : shapes) delete s;
    return 0;
}
