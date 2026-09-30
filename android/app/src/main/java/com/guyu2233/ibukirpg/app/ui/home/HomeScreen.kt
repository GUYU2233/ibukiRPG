package com.guyu2233.ibukirpg.app.ui.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.MenuBook
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.FolderOpen
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.guyu2233.ibukirpg.app.BuildConfig
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.SlotV1
import com.guyu2233.ibukirpg.app.ui.common.formatTime

@Composable
fun HomeScreen(vm: HomeViewModel, onGameReady: () -> Unit, onNewGame: () -> Unit, onSaves: () -> Unit, onSettings: () -> Unit) {
    val s by vm.state.collectAsStateWithLifecycle()
    val crash by vm.crash.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { vm.refresh() }
    crash?.let { CrashDialog(text = it, onDismiss = vm::dismissCrash) }
    HomeContent(
        s = s,
        onContinue = { vm.continueLatest(onGameReady) },
        onNewGame = onNewGame,
        onSaves = onSaves,
        onSettings = onSettings,
        onErrorShown = vm::dismissError,
    )
}

/** 无状态首页（便于截图测试与预览）。 */
@Composable
fun HomeContent(
    s: HomeState,
    onContinue: () -> Unit,
    onNewGame: () -> Unit,
    onSaves: () -> Unit,
    onSettings: () -> Unit,
    onErrorShown: () -> Unit,
) {
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(s.error) {
        s.error?.let { onErrorShown(); snackbar.showSnackbar(it) }
    }
    Scaffold(snackbarHost = { SnackbarHost(snackbar) }) { pad ->
        Box(Modifier.fillMaxSize().padding(pad), contentAlignment = Alignment.TopCenter) {
            Column(
                Modifier
                    .widthIn(max = 560.dp)
                    .fillMaxWidth()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 24.dp, vertical = 16.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Spacer(Modifier.height(40.dp))
                Icon(
                    Icons.AutoMirrored.Outlined.MenuBook, contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(56.dp),
                )
                Spacer(Modifier.height(12.dp))
                Text(
                    stringResource(R.string.app_name), style = MaterialTheme.typography.displaySmall,
                    color = MaterialTheme.colorScheme.onSurface, modifier = Modifier.semantics { heading() },
                )
                Text(stringResource(R.string.app_subtitle), style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.height(4.dp))
                Text(
                    stringResource(R.string.app_tagline), style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
                )
                Spacer(Modifier.height(12.dp))
                ModeChip(online = s.ai.online, model = s.ai.model, localModel = s.ai.kind == "llamacpp", onClick = onSettings)
                Spacer(Modifier.height(28.dp))

                if (s.loading) {
                    CircularProgressIndicator()
                    Text(stringResource(R.string.home_loading), Modifier.padding(top = 8.dp))
                } else {
                    s.latest?.let { LastSaveCard(it) }
                    Spacer(Modifier.height(16.dp))
                    if (s.latest != null) {
                        Button(
                            onClick = onContinue,
                            enabled = !s.working,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 64.dp),
                            shape = RoundedCornerShape(20.dp),
                            contentPadding = ButtonDefaults.ButtonWithIconContentPadding,
                        ) {
                            if (s.working) {
                                CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimary)
                            } else {
                                Icon(Icons.Outlined.PlayArrow, contentDescription = null)
                            }
                            Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                            Text(stringResource(R.string.home_continue), style = MaterialTheme.typography.titleMedium)
                        }
                        Spacer(Modifier.height(12.dp))
                        FilledTonalButton(
                            onClick = onNewGame, enabled = !s.working,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp),
                        ) {
                            Icon(Icons.Outlined.Add, contentDescription = null)
                            Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                            Text(stringResource(R.string.home_new_game))
                        }
                    } else {
                        Text(
                            stringResource(R.string.home_no_save), style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        Spacer(Modifier.height(16.dp))
                        Button(
                            onClick = onNewGame, enabled = !s.working,
                            modifier = Modifier.fillMaxWidth().heightIn(min = 64.dp), shape = RoundedCornerShape(20.dp),
                        ) {
                            Icon(Icons.Outlined.Add, contentDescription = null)
                            Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                            Text(stringResource(R.string.home_new_game), style = MaterialTheme.typography.titleMedium)
                        }
                    }
                    Spacer(Modifier.height(12.dp))
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        OutlinedButton(onClick = onSaves, modifier = Modifier.weight(1f).heightIn(min = 52.dp), enabled = !s.working) {
                            Icon(Icons.Outlined.FolderOpen, contentDescription = null)
                            Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                            Text(if (s.saveCount > 0) "${stringResource(R.string.home_saves)}（${s.saveCount}）" else stringResource(R.string.home_saves))
                        }
                        OutlinedButton(onClick = onSettings, modifier = Modifier.weight(1f).heightIn(min = 52.dp), enabled = !s.working) {
                            Icon(Icons.Outlined.Settings, contentDescription = null)
                            Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                            Text(stringResource(R.string.home_settings))
                        }
                    }
                }
                Spacer(Modifier.height(32.dp))
                Text(
                    stringResource(R.string.home_version, BuildConfig.VERSION_NAME),
                    style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline,
                )
            }
        }
    }
}

@Composable
private fun ModeChip(online: Boolean, model: String, localModel: Boolean, onClick: () -> Unit) {
    AssistChip(
        onClick = onClick,
        label = {
            Text(
                when {
                    localModel -> stringResource(R.string.home_mode_local_model, model)
                    online -> stringResource(R.string.home_mode_ai, model)
                    else -> stringResource(R.string.home_mode_offline)
                },
            )
        },
        leadingIcon = {
            Icon(
                if (online) Icons.Outlined.AutoAwesome else Icons.Outlined.CloudOff, contentDescription = null,
                modifier = Modifier.size(AssistChipDefaults.IconSize),
            )
        },
    )
}

@Composable
private fun LastSaveCard(slot: SlotV1) {
    ElevatedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(20.dp)) {
            Text(stringResource(R.string.home_last_played), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(6.dp))
            Text(slot.name, style = MaterialTheme.typography.titleLarge)
            if (slot.packName.isNotBlank()) {
                Text(
                    stringResource(R.string.saves_pack, slot.packName, slot.packVersion),
                    style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary,
                )
            }
            Spacer(Modifier.height(8.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.Place, contentDescription = null, modifier = Modifier.size(16.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.size(4.dp))
                Text(
                    "${slot.location} · ${slot.time}",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                buildString {
                    append(stringResource(R.string.saves_turn, slot.turn))
                    append(" · ").append(stringResource(R.string.game_gold, slot.gold))
                    slot.story?.takeIf { it.isNotBlank() }?.let { append(" · ").append(it) }
                },
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val t = formatTime(slot.updatedAt)
            if (t.isNotEmpty()) {
                Text(stringResource(R.string.saves_updated, t), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.outline)
            }
        }
    }
}

@Composable
fun NewGameDialog(packName: String, onDismiss: () -> Unit, onStart: (String) -> Unit) {
    val default = stringResource(R.string.new_game_name_default)
    var name by rememberSaveable { mutableStateOf(default) }
    val submit = { onStart(name.ifBlank { default }) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.new_game_title)) },
        text = {
            Column {
                Text(stringResource(R.string.new_game_hint, packName), style = MaterialTheme.typography.bodyMedium)
                Spacer(Modifier.height(16.dp))
                OutlinedTextField(
                    value = name, onValueChange = { if (it.length <= 12) name = it },
                    label = { Text(stringResource(R.string.new_game_name_label)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Go),
                    keyboardActions = KeyboardActions(onGo = { submit() }),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = { Button(onClick = submit) { Text(stringResource(R.string.new_game_start)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

/** 上次运行崩溃 / 被系统终止时的本地记录；可复制，关闭后删除。 */
@Composable
private fun CrashDialog(text: String, onDismiss: () -> Unit) {
    val clipboard = androidx.compose.ui.platform.LocalClipboardManager.current
    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.crash_title)) },
        text = {
            Column {
                Text(stringResource(R.string.crash_desc), style = MaterialTheme.typography.bodyMedium)
                Spacer(Modifier.height(8.dp))
                Box(Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
                    Text(text, style = MaterialTheme.typography.bodySmall, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace)
                }
            }
        },
        confirmButton = { androidx.compose.material3.TextButton(onClick = onDismiss) { Text(stringResource(R.string.crash_dismiss)) } },
        dismissButton = {
            androidx.compose.material3.TextButton(onClick = { clipboard.setText(androidx.compose.ui.text.AnnotatedString(text)) }) {
                Text(stringResource(R.string.crash_copy))
            }
        },
    )
}
