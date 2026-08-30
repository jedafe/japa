const BASE = 'http://localhost:3000';
const FINLAND = 'ff8b4b63-260c-4e2a-9e40-ec3dad7c2008';
const NIGERIA = 'ea5563ad-2319-4945-8829-11424872c28f';

const routes = [
  `/api/v1/credentials?dest=${FINLAND}`,
  `/api/v1/tax-calculator?dest=${FINLAND}&origin=${NIGERIA}&employment=employed`,
  `/api/v1/credentials/professions`,
];

for (const path of routes) {
  const res = await fetch(`${BASE}${path}`);
  const text = await res.text();
  console.log(`${path} -> ${res.status}`);
  console.log(text.slice(0, 500));
}
