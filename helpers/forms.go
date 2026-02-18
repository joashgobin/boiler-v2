package helpers

func FormPresets() map[string]string {
	presets := make(map[string]string)
	presets["name"] = `
	<div class="form-group">
	<label for="name">
	<input name="name" type="text">
	</div>
	`
	presets["password"] = `
	<div class="form-group">
	<label for="password">
	<input name="password" type="password">
	</div>
	`
	return presets
}
