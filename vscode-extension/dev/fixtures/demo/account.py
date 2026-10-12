class Account:
    def __init__(self, owner, balance):
        self.owner = owner
        self.balance = balance

    def deposit(self, amount):
        self.balance += amount


alice = Account("Alice", 100)
bob = Account("Bob", 50)
alice.deposit(25)
bob.deposit(10)
print(alice.balance, bob.balance)
