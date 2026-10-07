from collections import deque

graph = {
    'A': ['B', 'C'],
    'B': ['A', 'D', 'E'],
    'C': ['A', 'F'],
    'D': ['B'],
    'E': ['B', 'F'],
    'F': ['C', 'E'],
}


def bfs(start):
    visited = {start}
    queue = deque([start])
    order = []
    while queue:
        node = queue.popleft()
        order.append(node)
        for nb in graph[node]:
            if nb not in visited:
                visited.add(nb)
                queue.append(nb)
    return order


def dfs(node, visited):
    visited.add(node)
    print(node, end=' ')
    for nb in graph[node]:
        if nb not in visited:
            dfs(nb, visited)


def dfs_stack(start):
    seen = set()
    stack = [start]
    while stack:
        node = stack.pop()
        if node in seen:
            continue
        seen.add(node)
        for nb in reversed(graph[node]):
            if nb not in seen:
                stack.append(nb)
    return seen


print(bfs('A'))
dfs('A', set())
print()
print(sorted(dfs_stack('A')))
