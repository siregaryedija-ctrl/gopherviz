package irsim

import (
	"testing"

	"gopherviz/internal/lang"
	"gopherviz/internal/trace"
)

// TestTreeTraversal: rekursi node.left/node.right (object, dict, pointer struct) dan pohon umum
// (children) terbaca di semua bahasa; linked list (node.next) bukan pohon.
func TestTreeTraversal(t *testing.T) {
	cases := []struct {
		lang, file, src string
		line, nodes     int
	}{
		{"python", "bst.py", `class Node:
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
`, 9, 6},
		{"javascript", "t.js", `class Node {
  constructor(val, left = null, right = null) { this.val = val; this.left = left; this.right = right; }
}
const root = new Node(2, new Node(1), new Node(3));
function inorder(n, out) {
  if (n === null) return;
  inorder(n.left, out);
  out.push(n.val);
  inorder(n.right, out);
}
const hasil = [];
inorder(root, hasil);
console.log(hasil.join(" "));
`, 5, 3},
		{"java", "Pohon.java", `public class Pohon {
    static class TreeNode {
        int val; TreeNode left, right;
        TreeNode(int v) { val = v; }
    }
    static int jumlah(TreeNode node) {
        if (node == null) return 0;
        return node.val + jumlah(node.left) + jumlah(node.right);
    }
    public static void main(String[] args) {
        TreeNode r = new TreeNode(5);
        r.left = new TreeNode(3);
        r.right = new TreeNode(8);
        r.left.right = new TreeNode(4);
        System.out.println(jumlah(r));
    }
}
`, 6, 4},
		{"c", "t.c", `#include <stdio.h>
#include <stdlib.h>
struct Node { int data; struct Node *left; struct Node *right; };
struct Node *baru(int v) {
    struct Node *n = malloc(sizeof(struct Node));
    n->data = v; n->left = NULL; n->right = NULL;
    return n;
}
void postorder(struct Node *n) {
    if (n == NULL) return;
    postorder(n->left);
    postorder(n->right);
    printf("%d ", n->data);
}
int main(void) {
    struct Node *r = baru(1);
    r->left = baru(2);
    r->right = baru(3);
    postorder(r);
    printf("\n");
    return 0;
}
`, 9, 3},
		{"cpp", "t.cpp", `#include <iostream>
using namespace std;
struct Node {
    int val; Node *left = nullptr; Node *right = nullptr;
    Node(int v) : val(v) {}
};
int depth(Node *n) {
    if (!n) return 0;
    int a = depth(n->left), b = depth(n->right);
    return 1 + (a > b ? a : b);
}
int main() {
    Node *r = new Node(1);
    r->left = new Node(2);
    r->left->left = new Node(3);
    r->right = new Node(4);
    cout << depth(r) << endl;
    return 0;
}
`, 7, 4},
		{"php", "t.php", `<?php
class Node {
    public $val; public $left; public $right;
    function __construct($v, $l = null, $r = null) { $this->val = $v; $this->left = $l; $this->right = $r; }
}
function hitung($node) {
    if ($node === null) return 0;
    return 1 + hitung($node->left) + hitung($node->right);
}
echo hitung(new Node(1, new Node(2), new Node(3, new Node(4))));
`, 6, 4},
		{"python", "dict.py", `pohon = {"val": 1, "left": {"val": 2, "left": None, "right": None}, "right": {"val": 3, "left": None, "right": None}}

def cetak(n):
    if not n:
        return
    cetak(n["left"])
    print(n["val"])
    cetak(n["right"])

cetak(pohon)
`, 3, 3},
		{"python", "umum.py", `class Folder:
    def __init__(self, name, children=None):
        self.name = name
        self.children = children or []

def tampil(f, depth):
    print("  " * depth + f.name)
    for c in f.children:
        tampil(c, depth + 1)

tampil(Folder("root", [Folder("src", [Folder("main")]), Folder("docs")]), 0)
`, 6, 4},
		{"python", "linked.py", `class N:
    def __init__(self, v, nx=None):
        self.v = v
        self.next = nx

def total(n):
    if n is None:
        return 0
    return n.v + total(n.next)

print(total(N(1, N(2))))
`, 6, 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			res := lang.Analyze(c.lang, c.file, c.src)
			if len(res.Diagnostics) > 0 {
				t.Fatalf("diagnostik: %v", res.Diagnostics)
			}
			id, ok := res.Scene.BlockAt(c.line)
			if !ok {
				t.Fatalf("tidak ada blok di baris %d", c.line)
			}
			tr := Trace(res, id, Options{})
			if tr.Mode != trace.ModeSimulated {
				t.Fatalf("mode %s: %v", tr.Mode, tr.Warnings)
			}
			if c.nodes == 0 {
				if tr.Tree != nil {
					t.Errorf("linked list terbaca sebagai pohon: %+v", tr.Tree)
				}
				return
			}
			if tr.Tree == nil || len(tr.Tree.Nodes) != c.nodes {
				t.Fatalf("pohon %+v, mau %d simpul", tr.Tree, c.nodes)
			}
			seen := map[string]bool{}
			for _, s := range tr.Steps {
				if s.Node != nil && s.Node.Cur != "" {
					seen[s.Node.Cur] = true
				}
			}
			if len(seen) != c.nodes {
				t.Errorf("simpul dikunjungi %d, mau %d", len(seen), c.nodes)
			}
		})
	}
}
