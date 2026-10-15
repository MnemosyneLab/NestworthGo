/** UTF-8 wire/storage size, including replacement encoding for lone surrogates. */
export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}
