import math

values = [4, 8, 2, 9, 5, 7]
target = 9
found = -1
for i, value in enumerate(values):
    if value == target:
        found = i
        break
print(found)

arr = [1, 3, 5, 7, 9, 11, 13, 15, 17]
x = 13
lo, hi = 0, len(arr) - 1
while lo <= hi:
    mid = (lo + hi) // 2
    if arr[mid] == x:
        print("ketemu di", mid)
        break
    elif arr[mid] < x:
        lo = mid + 1
    else:
        hi = mid - 1

data = [2, 4, 6, 8, 10, 12, 14, 16, 18, 20]
cari = 14
n = len(data)
step = int(math.sqrt(n))
prev = 0
while data[min(step, n) - 1] < cari:
    prev = step
    step += int(math.sqrt(n))
    if prev >= n:
        break
while data[prev] < cari:
    prev += 1
    if prev == min(step, n):
        break
print("jump search:", prev if data[prev] == cari else -1)

nums = [10, 20, 30, 40, 50, 60, 70, 80]
key = 70
low, high = 0, len(nums) - 1
while low <= high and nums[low] <= key <= nums[high]:
    pos = low + (key - nums[low]) * (high - low) // (nums[high] - nums[low])
    if nums[pos] == key:
        print("interpolation:", pos)
        break
    if nums[pos] < key:
        low = pos + 1
    else:
        high = pos - 1

kosong = [3, 1, 4]
for v in kosong:
    if v == 99:
        print("ada")
        break
