/**
 * Selectable look-back windows, in hours.
 *
 * One hour is the default: the common question is "is that shower coming my way",
 * not "what happened on Tuesday". The longer windows are real — the backend keeps
 * the full week FMI itself retains — but a week of five-minute frames is ~2000
 * images, so it is opt-in rather than the landing state.
 */
export const WINDOWS = [
  { hours: 1, label: '1 h' },
  { hours: 3, label: '3 h' },
  { hours: 12, label: '12 h' },
  { hours: 24, label: '24 h' },
  { hours: 168, label: '7 vrk' },
];
