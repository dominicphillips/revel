Only place compilable (*.coffee, *.less) files in here. These files will be cross compiled into their respective folders in public/ and served by the static handler. Less partials should be prefixed with an underscore, e.g.

	"_colors.less"

then use it with the standard less import syntax

	@import "_colors.less";


Remember to set the path to the coffee and less binaries in app.conf if they are not in your $PATH. For example if you install from npm:

	$ npm install coffee-script less

in app.conf

	assets.coffee=node_modules/.bin/coffee
	assets.less=node_modules/.bin/lessc

