class Node:
    def __init__(self, val, left=None, right=None):
        self.val = val
        self.left = left
        self.right = right

root = Node(8, Node(3, Node(1), Node(6)), Node(10, None, Node(14)))

def preorder(node):
    if node is None:
        return
    print(node.val, end=" ")
    preorder(node.left)
    preorder(node.right)

def tinggi(node):
    if node is None:
        return 0
    return 1 + max(tinggi(node.left), tinggi(node.right))

preorder(root)
print()
print(tinggi(root))
