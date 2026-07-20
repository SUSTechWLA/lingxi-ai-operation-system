export function cycleFocusIndex(currentIndex: number, itemCount: number, reverse: boolean): number {
  if (itemCount <= 0) return -1
  if (currentIndex < 0 || currentIndex >= itemCount) return reverse ? itemCount - 1 : 0
  return reverse ? (currentIndex - 1 + itemCount) % itemCount : (currentIndex + 1) % itemCount
}
