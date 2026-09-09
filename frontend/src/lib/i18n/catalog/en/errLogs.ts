export const errLogs = {
  'errLogs.uploadTooLarge': 'The logs are too large even after compression — the server rejected the archive.',
  'errLogs.uploadRateLimited': 'Too many uploads in a row. Wait a bit and try again.',
  'errLogs.uploadNetwork': 'Could not reach the server. Check your internet connection.',
  'errLogs.uploadUnavailable': 'The server is not accepting logs right now. Try again later.',
  'errLogs.uploadRejected': 'The server did not accept this log archive.',
  'errLogs.uploadFailed': 'The server refused the logs.',
  'errLogs.uploadFallback': 'Failed to send the logs.',
} as const;
