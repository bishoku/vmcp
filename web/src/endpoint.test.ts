import { expect, test } from 'vitest'
import { connectionEndpoint } from './endpoint'

test('uses the configured gateway rather than the frontend host or a guessed port', () => {
  expect(connectionEndpoint('/mcp/v1/virtual/demo', 'http://localhost:8800/')).toBe('http://localhost:8800/mcp/v1/virtual/demo')
  expect(connectionEndpoint('/mcp/v1/virtual/demo', 'https://gateway.example.com/proxy/')).toBe('https://gateway.example.com/proxy/mcp/v1/virtual/demo')
})
test('does not advertise a full endpoint with absent or invalid public configuration', () => {
  for (const base of ['', '/relative', 'javascript:alert(1)', 'https://user:pass@example.com', 'https://example.com?token=x', 'https://example.com#fragment']) {
    expect(connectionEndpoint('/mcp/v1/virtual/demo', base)).toBeNull()
  }
})
