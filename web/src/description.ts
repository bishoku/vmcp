export const DESCRIPTION_LIMIT = 20_000
// Match OpenAPI maxLength and Go rune counts, including non-BMP Unicode.
export const descriptionLength = (text: string) => Array.from(text).length
