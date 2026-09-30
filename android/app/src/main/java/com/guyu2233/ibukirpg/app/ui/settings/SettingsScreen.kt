package com.guyu2233.ibukirpg.app.ui.settings

import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Slider
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlin.math.roundToInt
import com.guyu2233.ibukirpg.app.BuildConfig
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.ThemeMode

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(vm: SettingsViewModel, onBack: () -> Unit) {
    val modelPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(vm::importLocalModel)
    }
    val context = androidx.compose.ui.platform.LocalContext.current
    // Android 13+：本地模型生成时会显示前台服务通知，选择本地模型时顺带申请通知权限（拒绝也不影响使用）
    val notifPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    val settings by vm.settings.collectAsStateWithLifecycle()
    val form by vm.form.collectAsStateWithLifecycle()
    LaunchedEffect(form.kind) {
        if (form.kind == "llamacpp" && Build.VERSION.SDK_INT >= 33 &&
            androidx.core.content.ContextCompat.checkSelfPermission(context, android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED
        ) notifPermission.launch(android.Manifest.permission.POST_NOTIFICATIONS)
    }
    val saved by vm.saved.collectAsStateWithLifecycle()
    SettingsContent(
        settings = settings,
        form = form,
        saved = saved,
        engineVersion = vm.engineVersion,
        actions = SettingsActions(
            onBack = onBack,
            selectKind = vm::selectKind,
            setBaseUrl = vm::setBaseUrl,
            setModel = vm::setModel,
            setKey = vm::setKey,
            test = vm::test,
            save = vm::save,
            clearKey = vm::clearKey,
            importLocalModel = { modelPicker.launch(arrayOf("*/*")) },
            removeLocalModel = vm::removeLocalModel,
            setTemperature = vm::setTemperature,
            setContextTokens = vm::setContextTokens,
            setTopK = vm::setTopK,
            setTopP = vm::setTopP,
            setThreads = vm::setThreads,
            setGpuLayers = vm::setGpuLayers,
            setMaxTokens = vm::setMaxTokens,
            openBatterySettings = { BatteryHelp.open(context) },
            consumeSaved = vm::consumeSaved,
            setTextScale = { vm.setTextScale(it) },
            setTheme = { vm.setTheme(it) },
            setDynamic = { vm.setDynamic(it) },
        ),
    )
}

/** 设置页的全部回调。 */
data class SettingsActions(
    val onBack: () -> Unit = {},
    val selectKind: (String) -> Unit = {},
    val setBaseUrl: (String) -> Unit = {},
    val setModel: (String) -> Unit = {},
    val setKey: (String) -> Unit = {},
    val test: () -> Unit = {},
    val save: () -> Unit = {},
    val clearKey: () -> Unit = {},
    val importLocalModel: () -> Unit = {},
    val removeLocalModel: () -> Unit = {},
    val setTemperature: (Float) -> Unit = {},
    val setContextTokens: (Int) -> Unit = {},
    val setTopK: (Int) -> Unit = {},
    val setTopP: (Float) -> Unit = {},
    val setThreads: (Int) -> Unit = {},
    val setGpuLayers: (Int) -> Unit = {},
    val setMaxTokens: (Int) -> Unit = {},
    val openBatterySettings: () -> Unit = {},
    val consumeSaved: () -> Unit = {},
    val setTextScale: (Float) -> Unit = {},
    val setTheme: (ThemeMode) -> Unit = {},
    val setDynamic: (Boolean) -> Unit = {},
)

/** 无状态设置页（便于截图测试与预览）。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsContent(
    settings: com.guyu2233.ibukirpg.app.data.AppSettings,
    form: AIForm,
    saved: Boolean,
    engineVersion: String,
    actions: SettingsActions,
) {
    val vm = actions
    val onBack = actions.onBack
    val snackbar = remember { SnackbarHostState() }
    val savedMsg = stringResource(R.string.settings_saved)
    LaunchedEffect(saved) { if (saved) { vm.consumeSaved(); snackbar.showSnackbar(savedMsg) } }
    val scroll = TopAppBarDefaults.pinnedScrollBehavior()

    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.back)) }
                },
                scrollBehavior = scroll,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { pad ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(pad)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp)
                .navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Section(stringResource(R.string.settings_ai))
            Text(stringResource(R.string.settings_ai_desc), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer)) {
                Column(Modifier.selectableGroup()) {
                    ProviderOption("offline", stringResource(R.string.settings_offline), stringResource(R.string.settings_offline_desc), form.kind, vm.selectKind)
                    ProviderOption("llamacpp", stringResource(R.string.settings_local), stringResource(if (form.localAvailable) R.string.settings_local_desc else R.string.settings_local_unavailable), form.kind, vm.selectKind)
                    ProviderOption("deepseek", stringResource(R.string.settings_deepseek), "api.deepseek.com", form.kind, vm.selectKind)
                    ProviderOption("qwen", stringResource(R.string.settings_qwen), "DashScope 兼容模式", form.kind, vm.selectKind)
                    ProviderOption("custom", stringResource(R.string.settings_custom), null, form.kind, vm.selectKind)
                }
            }
            AnimatedVisibility(form.kind == "llamacpp") {
                LocalModelFields(form, vm)
            }
            AnimatedVisibility(form.kind != "offline" && form.kind != "llamacpp") {
                AIFields(form, vm)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.fillMaxWidth()) {
                if (form.kind != "offline") {
                    OutlinedButton(
                        onClick = vm.test,
                        enabled = form.test != TestState.Running && if (form.kind == "llamacpp") form.localModelPath.isNotBlank() else (form.key.isNotBlank() || form.hasSavedKey),
                        modifier = Modifier.weight(1f),
                    ) { Text(stringResource(R.string.settings_test)) }
                }
                Button(onClick = vm.save, enabled = !form.saving && form.dirty && (form.kind != "llamacpp" || form.localModelPath.isNotBlank()), modifier = Modifier.weight(1f)) {
                    Text(stringResource(R.string.settings_save))
                }
            }
            TestResult(form.test)

            Section(stringResource(R.string.settings_display))
            TextSizeSetting(settings.textScale, vm.setTextScale)
            Text(stringResource(R.string.settings_theme), style = MaterialTheme.typography.labelLarge)
            val modes = listOf(
                ThemeMode.SYSTEM to R.string.settings_theme_system,
                ThemeMode.LIGHT to R.string.settings_theme_light,
                ThemeMode.DARK to R.string.settings_theme_dark,
            )
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                modes.forEachIndexed { i, (m, label) ->
                    SegmentedButton(
                        selected = settings.theme == m,
                        onClick = { vm.setTheme(m) },
                        shape = SegmentedButtonDefaults.itemShape(i, modes.size),
                    ) { Text(stringResource(label)) }
                }
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                ListItem(
                    headlineContent = { Text(stringResource(R.string.settings_dynamic)) },
                    supportingContent = { Text(stringResource(R.string.settings_dynamic_desc)) },
                    trailingContent = { Switch(checked = settings.dynamicColor, onCheckedChange = null) },
                    modifier = Modifier.toggleable(value = settings.dynamicColor, role = Role.Switch, onValueChange = vm.setDynamic),
                    colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surface),
                )
            }

            Section(stringResource(R.string.settings_about))
            Text(
                stringResource(R.string.settings_about_text, BuildConfig.VERSION_NAME, engineVersion),
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(24.dp))
        }
    }
}

@Composable
private fun Section(title: String) {
    Text(
        title, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(top = 12.dp).semantics { heading() },
    )
}

@Composable
private fun ProviderOption(kind: String, title: String, subtitle: String?, selected: String, onSelect: (String) -> Unit) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = subtitle?.let { { Text(it) } },
        leadingContent = { RadioButton(selected = selected == kind, onClick = null) },
        colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
        modifier = Modifier.selectable(selected = selected == kind, role = Role.RadioButton, onClick = { onSelect(kind) }),
    )
}

@Composable
private fun LocalModelFields(form: AIForm, vm: SettingsActions) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(stringResource(R.string.settings_local_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        OutlinedButton(onClick = vm.importLocalModel, enabled = !form.importingModel && form.localAvailable, modifier = Modifier.fillMaxWidth()) {
            if (form.importingModel) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
            else Text(stringResource(R.string.settings_local_import))
        }
        if (form.localModelName.isNotBlank()) {
            ListItem(
                headlineContent = { Text(form.localModelName) },
                supportingContent = { Text(form.localStatus.ifBlank { stringResource(R.string.settings_local_local_only) }) },
                colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
                trailingContent = { TextButton(onClick = vm.removeLocalModel) { Text(stringResource(R.string.settings_local_remove)) } },
            )
        }
        form.memoryWarning?.let { Text(it, color = MaterialTheme.colorScheme.tertiary, style = MaterialTheme.typography.bodySmall) }
        form.modelError?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }

        Text(stringResource(R.string.settings_local_context, form.contextTokens), style = MaterialTheme.typography.labelLarge)
        Slider(
            value = form.contextTokens.toFloat(),
            onValueChange = { vm.setContextTokens((it / 512f).roundToInt().coerceIn(1, 16) * 512) },
            valueRange = 512f..8192f,
            steps = 14,
        )
        Text(stringResource(R.string.settings_local_max_tokens, form.maxTokens), style = MaterialTheme.typography.labelLarge)
        Slider(value = form.maxTokens.toFloat(), onValueChange = { vm.setMaxTokens((it / 32f).roundToInt() * 32) }, valueRange = 64f..1024f, steps = 29)
        Text(
            if (form.threads == 0) stringResource(R.string.settings_local_threads_auto) else stringResource(R.string.settings_local_threads, form.threads),
            style = MaterialTheme.typography.labelLarge,
        )
        Slider(value = form.threads.toFloat(), onValueChange = { vm.setThreads(it.roundToInt()) }, valueRange = 0f..8f, steps = 7)
        Text(stringResource(R.string.settings_local_temperature, form.temperature), style = MaterialTheme.typography.labelLarge)
        Slider(value = form.temperature, onValueChange = vm.setTemperature, valueRange = 0f..1.5f, steps = 14)
        Text(stringResource(R.string.settings_local_top_p, form.topP), style = MaterialTheme.typography.labelLarge)
        Slider(value = form.topP, onValueChange = vm.setTopP, valueRange = 0.1f..1f, steps = 17)
        Text(stringResource(R.string.settings_local_top_k, form.topK), style = MaterialTheme.typography.labelLarge)
        Slider(value = form.topK.toFloat(), onValueChange = { vm.setTopK(it.roundToInt()) }, valueRange = 1f..100f)
        Text(stringResource(R.string.settings_local_gpu_layers, form.gpuLayers), style = MaterialTheme.typography.labelLarge)
        Slider(value = form.gpuLayers.toFloat(), onValueChange = { vm.setGpuLayers(it.roundToInt()) }, valueRange = 0f..99f, enabled = false)
        Text(stringResource(R.string.settings_local_gpu_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(stringResource(R.string.settings_local_context_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        TextButton(onClick = vm.openBatterySettings) { Text(stringResource(R.string.settings_local_battery)) }
        Text(stringResource(R.string.settings_local_battery_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun AIFields(form: AIForm, vm: SettingsActions) {
    var show by rememberSaveable { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = form.baseUrl, onValueChange = vm.setBaseUrl, singleLine = true,
            label = { Text(stringResource(R.string.settings_base_url)) },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = form.model, onValueChange = vm.setModel, singleLine = true,
            label = { Text(stringResource(R.string.settings_model)) },
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = form.key, onValueChange = vm.setKey, singleLine = true,
            label = { Text(stringResource(R.string.settings_api_key)) },
            placeholder = { if (form.hasSavedKey) Text("••••••••") },
            supportingText = {
                Text(if (form.hasSavedKey) stringResource(R.string.settings_api_key_saved) else stringResource(R.string.settings_api_key_note))
            },
            visualTransformation = if (show) VisualTransformation.None else PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, autoCorrectEnabled = false),
            trailingIcon = {
                IconButton(onClick = { show = !show }) {
                    Icon(
                        if (show) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                        contentDescription = stringResource(if (show) R.string.settings_hide_key else R.string.settings_show_key),
                    )
                }
            },
            modifier = Modifier.fillMaxWidth(),
        )
        if (form.hasSavedKey) {
            TextButton(onClick = vm.clearKey) { Text(stringResource(R.string.settings_clear_key)) }
        }
    }
}

@Composable
private fun TestResult(t: TestState) {
    when (t) {
        TestState.Idle -> Unit
        TestState.Running -> Row(verticalAlignment = Alignment.CenterVertically) {
            CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
            Spacer(Modifier.size(8.dp))
            Text(stringResource(R.string.settings_testing))
        }
        is TestState.Ok -> Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.CheckCircle, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.size(8.dp))
            Text(stringResource(R.string.settings_test_ok, t.ms.toInt()), color = MaterialTheme.colorScheme.primary)
        }
        is TestState.Fail -> Row(verticalAlignment = Alignment.Top) {
            Icon(Icons.Outlined.ErrorOutline, contentDescription = null, tint = MaterialTheme.colorScheme.error)
            Spacer(Modifier.size(8.dp))
            Text(stringResource(R.string.settings_test_fail, t.msg), color = MaterialTheme.colorScheme.error)
        }
    }
}

@Composable
private fun TextSizeSetting(value: Float, onChange: (Float) -> Unit) {
    var v by remember(value) { mutableFloatStateOf(value) }
    Column {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.settings_text_size), style = MaterialTheme.typography.labelLarge, modifier = Modifier.weight(1f))
            Text("${(v * 100).toInt()}%", style = MaterialTheme.typography.labelLarge)
        }
        Slider(
            value = v, onValueChange = { v = it }, onValueChangeFinished = { onChange(v) },
            valueRange = 0.85f..1.4f, steps = 10,
        )
        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
            Text(
                stringResource(R.string.settings_text_preview),
                style = MaterialTheme.typography.bodyLarge.copy(fontSize = 16.sp * (v / value.coerceAtLeast(0.1f))),
                modifier = Modifier.padding(12.dp),
            )
        }
    }
}
