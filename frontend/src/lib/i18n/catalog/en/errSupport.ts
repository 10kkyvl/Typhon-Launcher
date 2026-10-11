export const errSupport = {
  'errSupport.notFound': 'Request not found.',
  'errSupport.invalidTopic': 'Choose a topic for your request.',
  'errSupport.invalidSubject': 'The subject must be a single line of 1 to 120 characters.',
  'errSupport.invalidBody': 'The message must be 1 to 5000 characters.',
  'errSupport.invalidGame': 'The game could not be identified.',
  'errSupport.invalidClient': 'Launcher details could not be attached.',
  'errSupport.limit': 'Too many open requests or messages. Wait for support to answer.',
  'errSupport.bundleLimit': 'Too many log archives are attached to your requests. Send the message without logs.',
  'errSupport.bundleUnavailable': 'This log archive cannot be attached. Send the logs again.',
  'errSupport.logsUnavailable': 'Sending logs is unavailable right now. Send the request without them.',
  'errSupport.logsFailed': 'The logs could not be sent, so nothing was submitted. Try again or send without logs.',
  'errSupport.generic': 'The action failed. Try again later.',
} as const;
