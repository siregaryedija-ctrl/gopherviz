#include <iostream>
#include <limits>
#include <string>
using namespace std;

int main() {
    int choice;
    string name;
    do {
        cout << "1. Add  2. Show  0. Quit\nChoice: ";
        if (!(cin >> choice)) {
            cin.clear();
            cin.ignore(numeric_limits<streamsize>::max(), '\n');
            cout << "Please type a number.\n";
            continue;
        }
        cin.ignore(numeric_limits<streamsize>::max(), '\n');
        switch (choice) {
            case 1:
                cout << "Name: ";
                getline(cin, name);
                break;
            case 2:
                cout << "Last name: " << name << "\n";
                break;
            case 0:
                break;
            default:
                cout << "Unknown menu.\n";
        }
    } while (choice != 999);
    return 0;
}
