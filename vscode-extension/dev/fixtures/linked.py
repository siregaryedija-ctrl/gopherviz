class Node:
    def __init__(self, val):
        self.val = val
        self.next = None

head = Node(1)
head.next = Node(2)
head.next.next = Node(3)
total = 0
n = head
while n:
    total += n.val
    n = n.next
print(total)
