// Shared label map for the JSON editors — host apps pass localized strings,
// English defaults are used when a label is omitted.
export interface JsonEditorLabels {
	addField?: string;
	addItem?: string;
	remove?: string;
	moveUp?: string;
	moveDown?: string;
	key?: string;
	value?: string;
	empty?: string;
	typeString?: string;
	typeNumber?: string;
	typeBoolean?: string;
	typeNull?: string;
	typeObject?: string;
	typeArray?: string;
	startObject?: string;
	startArray?: string;
	startText?: string;
	startNumber?: string;
	startBoolean?: string;
	formMode?: string;
	rawMode?: string;
	invalidJson?: string;
}
