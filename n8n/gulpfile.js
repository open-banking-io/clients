const path = require('path');
const { src, dest } = require('gulp');

// Copy node/credential icons and codex (*.node.json) into dist, preserving their
// folder layout so the `file:<icon>.svg` references and codex metadata resolve at runtime.
function buildIcons() {
	const nodeSource = path.resolve('nodes', '**', '*.{png,svg,json}');
	const nodeDestination = path.resolve('dist', 'nodes');
	src(nodeSource).pipe(dest(nodeDestination));

	const credSource = path.resolve('credentials', '**', '*.{png,svg}');
	const credDestination = path.resolve('dist', 'credentials');
	return src(credSource, { allowEmpty: true }).pipe(dest(credDestination));
}

exports['build:icons'] = buildIcons;
