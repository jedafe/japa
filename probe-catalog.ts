const BASE = 'http://localhost:3000';
const names = [
  'tax_calculator.employment_status',
  'cv_formatter.format',
  'remittance.alert_direction',
  'appointment_alert.notification_channels',
];

for (const name of names) {
  const res = await fetch(`${BASE}/api/v1/catalogs/${encodeURIComponent(name)}`);
  const text = await res.text();
  console.log(`${name} -> ${res.status}`);
  console.log(text.slice(0, 1000));
}
