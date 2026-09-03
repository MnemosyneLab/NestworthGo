/** paginateActivityPage mirrors the Go keyset: effectiveAt DESC, createdAt DESC, id DESC. */
export function paginateActivityPage<T extends { id: string; effectiveAt?: string; createdAt?: string }>(
  activities: readonly T[],
  request: { afterId?: string; afterEffectiveAt?: string; afterCreatedAt?: string; limit?: number } = {},
): { activities: T[]; hasMore: boolean; next?: { id: string; effectiveAt: string; createdAt: string } } {
  const limit = request.limit && request.limit > 0 ? request.limit : 50;
  const sorted = [...activities].sort((left, right) => {
    const effective = (right.effectiveAt ?? "").localeCompare(left.effectiveAt ?? "");
    if (effective !== 0) {
      return effective;
    }
    const created = (right.createdAt ?? "").localeCompare(left.createdAt ?? "");
    if (created !== 0) {
      return created;
    }
    return right.id.localeCompare(left.id);
  });
  const afterIndex = request.afterId ? sorted.findIndex((activity) => activity.id === request.afterId) : -1;
  const start = request.afterId ? (afterIndex >= 0 ? afterIndex + 1 : sorted.length) : 0;
  const page = sorted.slice(start, start + limit);
  const hasMore = start + page.length < sorted.length;
  const last = page[page.length - 1];
  return {
    activities: page,
    hasMore,
    ...(hasMore && last
      ? { next: { id: last.id, effectiveAt: last.effectiveAt ?? "", createdAt: last.createdAt ?? "" } }
      : {}),
  };
}
