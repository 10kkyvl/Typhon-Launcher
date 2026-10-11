export const SUPPORT_CODES = [
  'ticket_not_found',
  'ticket_invalid_topic',
  'ticket_invalid_subject',
  'ticket_invalid_body',
  'ticket_invalid_game',
  'ticket_invalid_client',
  'ticket_limit',
  'ticket_bundle_limit',
  'ticket_bundle_unavailable',
] as const;

export const SUPPORT_CODE_FIELDS: Record<string, string> = {
  ticket_invalid_topic: 'topic',
  ticket_invalid_subject: 'subject',
  ticket_invalid_body: 'body',
  ticket_invalid_game: 'game',
  ticket_invalid_client: 'client',
  ticket_bundle_limit: 'logs',
  ticket_bundle_unavailable: 'logs',
};
