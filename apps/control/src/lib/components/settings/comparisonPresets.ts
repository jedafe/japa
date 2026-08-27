// UI presets for the comparison.auto_dimension_hints editor.
//
// These are starter templates derived from JapaConnect's pre-seeded comparison
// types and tools (§30, §70-§74, §90-§101). Admins can add one or all presets
// and then edit them; the actual configured value is still persisted through the
// system settings API per Rule 16.
//
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md

export interface ComparisonHintPreset {
	name: string;
	hints: string[];
	dimensions: string[];
	prepend?: boolean;
}

export const comparisonHintPresets: ComparisonHintPreset[] = [
	{
		name: 'Price & cost',
		hints: ['price', 'cost', 'plan', 'package', 'version', 'edition', 'model', 'fee', 'rate'],
		dimensions: ['Price', 'Value for money', 'Fees', 'Discounts'],
		prepend: true
	},
	{
		name: 'University & course',
		hints: ['course', 'university', 'school', 'program', 'degree', 'certification', 'academic', 'institution', 'college'],
		dimensions: ['Curriculum', 'Duration', 'Cost', 'Accreditation', 'Location', 'Ranking'],
		prepend: false
	},
	{
		name: 'Visa & immigration',
		hints: ['visa', 'immigration', 'work permit', 'residency', 'pr', 'permanent residency', 'green card', 'route', 'pathway', 'work visa', 'study visa'],
		dimensions: ['Eligibility', 'Processing time', 'Cost', 'Validity', 'Requirements', 'Pathway to PR'],
		prepend: false
	},
	{
		name: 'Cost of living',
		hints: ['cost of living', 'col', 'living', 'rent', 'expenses', 'budget', 'accommodation cost', 'monthly cost'],
		dimensions: ['Rent', 'Food', 'Transport', 'Utilities', 'Healthcare', 'Total monthly cost'],
		prepend: false
	},
	{
		name: 'Bank & finance',
		hints: ['bank', 'account', 'savings', 'current', 'debit', 'credit', 'card', 'transfer', 'finance', 'loan', 'mortgage'],
		dimensions: ['Fees', 'Interest rate', 'Minimum balance', 'Transfer fees', 'ATM access', 'Customer support'],
		prepend: false
	},
	{
		name: 'Mentor & coaching',
		hints: ['mentor', 'mentorship', 'coaching', 'guide', 'advisor', 'tutor', 'counsellor'],
		dimensions: ['Experience', 'Availability', 'Cost', 'Specialization', 'Rating', 'Success rate'],
		prepend: false
	},
	{
		name: 'Country / state / city',
		hints: ['country', 'state', 'city', 'destination', 'location', 'place', 'region', 'province'],
		dimensions: ['Cost of living', 'Safety', 'Climate', 'Job market', 'Healthcare', 'Education', 'Culture'],
		prepend: false
	},
	{
		name: 'Job & career',
		hints: ['job', 'salary', 'employer', 'career', 'role', 'position', 'hiring', 'recruitment', 'vacancy'],
		dimensions: ['Salary', 'Benefits', 'Remote policy', 'Growth', 'Location', 'Company rating'],
		prepend: false
	},
	{
		name: 'Housing & rent',
		hints: ['housing', 'apartment', 'rent', 'flat', 'accommodation', 'hostel', 'dorm', 'property', 'lease'],
		dimensions: ['Rent', 'Location', 'Amenities', 'Deposit', 'Lease terms', 'Utilities included'],
		prepend: false
	},
	{
		name: 'Healthcare & insurance',
		hints: ['health', 'insurance', 'hospital', 'clinic', 'healthcare', 'medical', 'pharmacy', 'coverage'],
		dimensions: ['Coverage', 'Premium', 'Network', 'Deductible', 'Claim process', 'Accessibility'],
		prepend: false
	},
	{
		name: 'Language test',
		hints: ['ielts', 'toefl', 'gre', 'gmat', 'language test', 'exam', 'english test', 'pte', 'duolingo'],
		dimensions: ['Cost', 'Validity', 'Accepted by', 'Format', 'Score range', 'Preparation time'],
		prepend: false
	},
	{
		name: 'Scholarship & funding',
		hints: ['scholarship', 'grant', 'bursary', 'funding', 'financial aid', 'tuition waiver', 'award', 'fellowship'],
		dimensions: ['Amount', 'Eligibility', 'Deadline', 'Coverage', 'Renewal', 'Application effort'],
		prepend: false
	},
	{
		name: 'Remittance & transfer',
		hints: ['remittance', 'money transfer', 'send money', 'exchange', 'fx', 'rate', 'wire', 'transfer', 'currency'],
		dimensions: ['Exchange rate', 'Fees', 'Speed', 'Limits', 'Payout options', 'Trust'],
		prepend: false
	},
	{
		name: 'Mobile & internet',
		hints: ['mobile', 'sim', 'data plan', 'internet', 'broadband', 'network', 'mtn', 'airtel', 'glo', '9mobile'],
		dimensions: ['Cost', 'Data allowance', 'Speed', 'Coverage', 'Contract', 'Customer service'],
		prepend: false
	},
	{
		name: 'Tax & compliance',
		hints: ['tax', 'taxation', 'income tax', 'vat', 'gst', 'double taxation', 'filing', 'deductions'],
		dimensions: ['Rate', 'Filing', 'Deductions', 'Treaty', 'Complexity', 'Residency rules'],
		prepend: false
	},
	{
		name: 'Credential & attestation',
		hints: ['credential', 'attestation', 'certificate', 'transcript', 'wes', 'evaluation', 'authentication', 'apostille', 'notary'],
		dimensions: ['Cost', 'Processing time', 'Validity', 'Requirements', 'Acceptance', 'Steps'],
		prepend: false
	},
	{
		name: 'Embassy & consulate',
		hints: ['embassy', 'consulate', 'diplomatic mission', 'appointment', 'visa office', 'high commission'],
		dimensions: ['Location', 'Appointment availability', 'Processing time', 'Contact', 'Services', 'Wait time'],
		prepend: false
	},
	{
		name: 'Travel & flights',
		hints: ['flight', 'airline', 'travel', 'trip', 'ticket', 'airfare', 'airport', 'layover', 'stopover'],
		dimensions: ['Price', 'Duration', 'Stops', 'Baggage', 'Airline rating', 'Refund policy'],
		prepend: false
	},
	{
		name: 'Recruitment & agent',
		hints: ['agent', 'recruiter', 'consultant', 'migration agent', 'travel agent', 'agency', 'broker', 'middleman'],
		dimensions: ['Fee', 'Reputation', 'Services', 'Success rate', 'License', 'Reviews'],
		prepend: false
	}
];
