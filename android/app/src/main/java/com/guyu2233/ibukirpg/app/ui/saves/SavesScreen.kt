package com.guyu2233.ibukirpg.app.ui.saves

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.DriveFileRenameOutline
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material.icons.outlined.FolderOff
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.SuggestionChip
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.SlotV1
import com.guyu2233.ibukirpg.app.ui.common.formatTime

@Composable
fun SavesScreen(vm: SavesViewModel, onBack: () -> Unit, onGameReady: () -> Unit) {
    val s by vm.state.collectAsStateWithLifecycle()
    var exporting by remember { mutableStateOf<java.io.File?>(null) }
    val createDoc = androidx.activity.compose.rememberLauncherForActivityResult(androidx.activity.result.contract.ActivityResultContracts.CreateDocument("application/zip")) { uri ->
        val f = exporting
        exporting = null
        if (uri != null && f != null) vm.writeExport(f, uri) else f?.delete()
    }
    val openDoc = androidx.activity.compose.rememberLauncherForActivityResult(androidx.activity.result.contract.ActivityResultContracts.OpenDocument()) { uri ->
        uri?.let(vm::import)
    }
    SavesContent(
        onExport = { slot -> vm.export(slot) { file, name -> exporting = file; createDoc.launch(name) } },
        onImport = { openDoc.launch(arrayOf("*/*")) },
        s = s,
        onBack = onBack,
        onOpen = { vm.load(it, onGameReady) },
        onCopy = { slot, msg -> vm.copy(slot, msg) },
        onRename = { slot, name -> vm.rename(slot, name) },
        onDelete = { slot, msg -> vm.delete(slot, msg) },
        onMessageShown = vm::consumeMessage,
    )
}

/** 无状态的存档列表（便于截图测试）。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SavesContent(
    s: SavesState,
    onBack: () -> Unit,
    onOpen: (SlotV1) -> Unit,
    onCopy: (SlotV1, String) -> Unit,
    onRename: (SlotV1, String) -> Unit,
    onDelete: (SlotV1, String) -> Unit,
    onMessageShown: () -> Unit,
    onExport: (SlotV1) -> Unit = {},
    onImport: () -> Unit = {},
) {
    val snackbar = remember { SnackbarHostState() }
    var confirmDelete by remember { mutableStateOf<SlotV1?>(null) }
    var renaming by remember { mutableStateOf<SlotV1?>(null) }
    val deletedMsg = stringResource(R.string.saves_deleted)
    val copiedMsg = stringResource(R.string.saves_copied)
    LaunchedEffect(s.message) { s.message?.let { snackbar.showSnackbar(it); onMessageShown() } }
    val scroll = TopAppBarDefaults.pinnedScrollBehavior()

    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.saves_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    androidx.compose.material3.TextButton(onClick = onImport, enabled = !s.working) { Text("导入存档") }
                },
                scrollBehavior = scroll,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { pad ->
        Box(Modifier.fillMaxSize().padding(pad)) {
            if (s.working) LinearProgressIndicator(Modifier.fillMaxWidth())
            when {
                s.loading -> CircularProgressIndicator(Modifier.align(Alignment.Center))
                s.slots.isEmpty() -> Column(Modifier.align(Alignment.Center), horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Outlined.FolderOff, contentDescription = null, modifier = Modifier.size(48.dp), tint = MaterialTheme.colorScheme.outline)
                    Spacer(Modifier.height(8.dp))
                    Text(stringResource(R.string.saves_empty), color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                else -> LazyColumn(
                    contentPadding = PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    items(s.slots, key = { it.id }) { slot ->
                        SlotCard(
                            slot = slot,
                            enabled = !s.working,
                            onOpen = { onOpen(slot) },
                            onCopy = { onCopy(slot, copiedMsg) },
                            onRename = { renaming = slot },
                            onDelete = { confirmDelete = slot },
                            onExport = { onExport(slot) },
                        )
                    }
                }
            }
        }
    }

    confirmDelete?.let { slot ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            icon = { Icon(Icons.Outlined.Delete, contentDescription = null) },
            title = { Text(stringResource(R.string.saves_delete_title)) },
            text = { Text(stringResource(R.string.saves_delete_msg, slot.name)) },
            confirmButton = {
                Button(
                    onClick = { confirmDelete = null; onDelete(slot, deletedMsg) },
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error, contentColor = MaterialTheme.colorScheme.onError),
                ) { Text(stringResource(R.string.saves_delete)) }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    renaming?.let { slot ->
        var name by remember(slot.id) { mutableStateOf(slot.name) }
        AlertDialog(
            onDismissRequest = { renaming = null },
            title = { Text(stringResource(R.string.saves_rename_title)) },
            text = {
                OutlinedTextField(value = name, onValueChange = { if (it.length <= 24) name = it }, singleLine = true, modifier = Modifier.fillMaxWidth())
            },
            confirmButton = {
                Button(onClick = { renaming = null; onRename(slot, name) }, enabled = name.isNotBlank()) { Text(stringResource(R.string.confirm)) }
            },
            dismissButton = { TextButton(onClick = { renaming = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun SlotCard(
    slot: SlotV1,
    enabled: Boolean,
    onOpen: () -> Unit,
    onCopy: () -> Unit,
    onRename: () -> Unit,
    onDelete: () -> Unit,
    onExport: () -> Unit = {},
) {
    var menu by remember { mutableStateOf(false) }
    Card(onClick = onOpen, enabled = enabled, modifier = Modifier.fillMaxWidth()) {
        Row(Modifier.padding(start = 20.dp, top = 16.dp, bottom = 16.dp, end = 4.dp), verticalAlignment = Alignment.Top) {
            Column(Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(slot.name, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f, fill = false))
                    if (slot.current) {
                        Spacer(Modifier.size(8.dp))
                        SuggestionChip(onClick = {}, label = { Text(stringResource(R.string.saves_current)) }, enabled = false)
                    }
                }
                if (slot.packName.isNotBlank() || slot.packId.isNotBlank()) {
                    Text(
                        stringResource(R.string.saves_pack, slot.packName.ifBlank { slot.packId }, slot.packVersion),
                        style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary,
                    )
                }
                Text(
                    "${slot.location} · ${slot.time}",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    buildString {
                        append(stringResource(R.string.saves_turn, slot.turn))
                        append(" · ").append(stringResource(R.string.game_gold, slot.gold))
                        slot.story?.takeIf { it.isNotBlank() }?.let { append(" · ").append(it) }
                    },
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    stringResource(R.string.saves_updated, formatTime(slot.updatedAt)),
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.outline,
                )
                slot.packProblem?.takeIf { it.isNotBlank() }?.let { problem ->
                    Row(Modifier.padding(top = 4.dp), verticalAlignment = Alignment.Top) {
                        Icon(Icons.Outlined.ErrorOutline, contentDescription = null, tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.size(4.dp))
                        Text(stringResource(R.string.saves_pack_problem, problem), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                    }
                }
            }
            Box {
                IconButton(onClick = { menu = true }, enabled = enabled) {
                    Icon(Icons.Outlined.MoreVert, contentDescription = stringResource(R.string.saves_more) + "：" + slot.name)
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.saves_copy)) },
                        leadingIcon = { Icon(Icons.Outlined.ContentCopy, contentDescription = null) },
                        onClick = { menu = false; onCopy() },
                    )
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.saves_rename)) },
                        leadingIcon = { Icon(Icons.Outlined.DriveFileRenameOutline, contentDescription = null) },
                        onClick = { menu = false; onRename() },
                    )
                    DropdownMenuItem(
                        text = { Text("导出（.ibksave）") },
                        leadingIcon = { Icon(Icons.Outlined.IosShare, contentDescription = null) },
                        onClick = { menu = false; onExport() },
                    )
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.saves_delete), color = MaterialTheme.colorScheme.error) },
                        leadingIcon = { Icon(Icons.Outlined.Delete, contentDescription = null, tint = MaterialTheme.colorScheme.error) },
                        onClick = { menu = false; onDelete() },
                    )
                }
            }
        }
    }
}
