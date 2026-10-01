package com.guyu2233.ibukirpg.app.ui.packs

import android.graphics.BitmapFactory
import android.util.Base64
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.MenuBook
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material.icons.outlined.FileOpen
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material.icons.outlined.TipsAndUpdates
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.PackV1
import com.guyu2233.ibukirpg.app.ui.common.parseAccent
import com.guyu2233.ibukirpg.app.ui.common.symbolFor

/** SAF 文件选择器的类型：部分文件管理器把 .zip 报成 octet-stream。 */
private val ZIP_TYPES = arrayOf("application/zip", "application/x-zip-compressed", "application/octet-stream")

@Composable
fun PacksScreen(vm: PacksViewModel, onBack: () -> Unit, onGameReady: () -> Unit) {
    val s by vm.state.collectAsStateWithLifecycle()
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) vm.import(uri)
    }
    val deletedMsg = stringResource(R.string.packs_deleted)
    PacksContent(
        s = s,
        onBack = onBack,
        onImport = { picker.launch(ZIP_TYPES) },
        onStart = vm::openCreation,
        onDelete = { vm.delete(it, deletedMsg) },
        onMessageShown = vm::consumeMessage,
        onDismissImportError = vm::dismissImportError,
    )
    s.creation?.let { cs ->
        CreationDialog(
            state = cs, working = s.working, defaultName = stringResource(R.string.new_game_name_default),
            onDismiss = vm::closeCreation, onReview = vm::reviewCreation, onClearReview = vm::clearReview,
            onStart = { name, c -> vm.newGame(cs.pack, name, c, onGameReady) },
        )
    }
}

/** 无状态的故事包选择界面（便于截图测试）。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PacksContent(
    s: PacksState,
    onBack: () -> Unit,
    onImport: () -> Unit,
    onStart: (PackV1) -> Unit,
    onDelete: (PackV1) -> Unit,
    onMessageShown: () -> Unit,
    onDismissImportError: () -> Unit,
) {
    val snackbar = remember { SnackbarHostState() }
    var deleting by remember { mutableStateOf<PackV1?>(null) }
    val importedMsg = s.imported?.let { r ->
        if (r.replaced) stringResource(R.string.packs_updated, r.pack.name, r.previousVersion ?: "?", r.pack.version)
        else stringResource(R.string.packs_imported, r.pack.name, r.pack.version)
    }
    LaunchedEffect(s.message, importedMsg) {
        val m = importedMsg ?: s.message ?: return@LaunchedEffect
        snackbar.showSnackbar(m)
        onMessageShown()
    }
    val scroll = TopAppBarDefaults.pinnedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scroll.nestedScrollConnection),
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.packs_title))
                        Text(
                            stringResource(R.string.packs_subtitle), style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.back))
                    }
                },
                actions = {
                    TextButton(onClick = onImport, enabled = !s.working) {
                        Icon(Icons.Outlined.FileOpen, contentDescription = null, modifier = Modifier.size(ButtonDefaults.IconSize))
                        Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                        Text(stringResource(R.string.packs_import))
                    }
                },
                scrollBehavior = scroll,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { pad ->
        Box(Modifier.fillMaxSize().padding(pad), contentAlignment = Alignment.TopCenter) {
            when {
                s.loading -> CircularProgressIndicator(Modifier.align(Alignment.Center))
                else -> LazyColumn(
                    modifier = Modifier.widthIn(max = 640.dp).fillMaxSize(),
                    contentPadding = PaddingValues(16.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    if (s.importing) {
                        item(key = "importing") {
                            Column {
                                Text(stringResource(R.string.packs_importing), style = MaterialTheme.typography.bodyMedium)
                                LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 8.dp))
                            }
                        }
                    }
                    if (s.packs.isEmpty()) {
                        item(key = "empty") { Text(stringResource(R.string.packs_empty), color = MaterialTheme.colorScheme.onSurfaceVariant) }
                    }
                    items(s.packs, key = { it.id }) { p ->
                        PackCard(
                            pack = p,
                            enabled = !s.working,
                            onStart = { onStart(p) },
                            onDelete = { deleting = p },
                        )
                    }
                    item(key = "more") { MoreCard(onImport, enabled = !s.working) }
                }
            }
        }
    }

    deleting?.let { p ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            icon = { Icon(Icons.Outlined.Delete, contentDescription = null) },
            title = { Text(stringResource(R.string.packs_delete_title)) },
            text = {
                Text(
                    if (p.saveCount > 0) stringResource(R.string.packs_delete_msg_saves, p.name, p.saveCount)
                    else stringResource(R.string.packs_delete_msg, p.name),
                )
            },
            confirmButton = {
                Button(
                    onClick = { deleting = null; onDelete(p) },
                    colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error, contentColor = MaterialTheme.colorScheme.onError),
                ) { Text(stringResource(R.string.saves_delete)) }
            },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
    s.importError?.let { err ->
        AlertDialog(
            onDismissRequest = onDismissImportError,
            icon = { Icon(Icons.Outlined.ErrorOutline, contentDescription = null, tint = MaterialTheme.colorScheme.error) },
            title = { Text(stringResource(R.string.packs_import_failed)) },
            text = { Text(err) },
            confirmButton = { TextButton(onClick = onDismissImportError) { Text(stringResource(R.string.confirm)) } },
        )
    }
}

@Composable
private fun PackCover(pack: PackV1) {
    val accent = parseAccent(pack.accent) ?: MaterialTheme.colorScheme.primary
    val bitmap = remember(pack.id, pack.cover) {
        if (pack.cover.isBlank()) null else runCatching {
            val bytes = Base64.decode(pack.cover, Base64.DEFAULT)
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
        }.getOrNull()
    }
    Box(
        Modifier
            .fillMaxWidth()
            .height(112.dp)
            .background(Brush.linearGradient(listOf(accent, accent.copy(alpha = 0.55f)))),
    ) {
        if (bitmap != null) {
            Image(bitmap, contentDescription = null, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
            Box(Modifier.fillMaxSize().background(Brush.verticalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = 0.55f)))))
        } else {
            Icon(
                symbolFor(pack.icon, Icons.AutoMirrored.Outlined.MenuBook), contentDescription = null,
                tint = Color.White.copy(alpha = 0.28f),
                modifier = Modifier.align(Alignment.CenterEnd).padding(end = 20.dp).size(80.dp),
            )
        }
        Column(Modifier.align(Alignment.BottomStart).padding(16.dp)) {
            Surface(color = Color.White.copy(alpha = 0.22f), contentColor = Color.White, shape = MaterialTheme.shapes.extraSmall) {
                Text(
                    stringResource(if (pack.builtin) R.string.packs_builtin else R.string.packs_user),
                    style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                )
            }
            Spacer(Modifier.height(4.dp))
            Text(
                pack.name, style = MaterialTheme.typography.headlineSmall, color = Color.White,
                maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.semantics { heading() },
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun PackCard(pack: PackV1, enabled: Boolean, onStart: () -> Unit, onDelete: () -> Unit) {
    var expanded by rememberSaveable(pack.id) { mutableStateOf(false) }
    var overflow by remember(pack.id) { mutableStateOf(false) }
    ElevatedCard(Modifier.fillMaxWidth()) {
        PackCover(pack)
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (pack.tagline.isNotBlank()) {
                Text(pack.tagline, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
            }
            if (pack.description.isNotBlank()) {
                Text(
                    pack.description.trim(), style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = if (expanded) Int.MAX_VALUE else 3, overflow = TextOverflow.Ellipsis,
                    onTextLayout = { if (!expanded) overflow = it.hasVisualOverflow },
                )
                if (!expanded && overflow) {
                    TextButton(onClick = { expanded = true }, contentPadding = PaddingValues(0.dp), modifier = Modifier.heightIn(min = 32.dp)) {
                        Text(stringResource(R.string.packs_read_more), style = MaterialTheme.typography.labelLarge)
                    }
                }
            }
            if (pack.tags.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    pack.tags.forEach { t ->
                        Surface(color = MaterialTheme.colorScheme.secondaryContainer, shape = MaterialTheme.shapes.small) {
                            Text(t, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp))
                        }
                    }
                }
            }
            Text(
                buildList {
                    if (pack.author.isNotBlank()) add(stringResource(R.string.packs_author, pack.author))
                    add(stringResource(R.string.packs_version, pack.version))
                    if (pack.saveCount > 0) add(stringResource(R.string.packs_saves, pack.saveCount))
                }.joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.outline,
            )
            pack.error?.takeIf { it.isNotBlank() }?.let { err ->
                Surface(color = MaterialTheme.colorScheme.errorContainer, contentColor = MaterialTheme.colorScheme.onErrorContainer, shape = MaterialTheme.shapes.small) {
                    Row(Modifier.padding(10.dp), verticalAlignment = Alignment.Top) {
                        Icon(Icons.Outlined.ErrorOutline, contentDescription = null, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.size(8.dp))
                        Text(stringResource(R.string.packs_unavailable, err), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Button(onClick = onStart, enabled = enabled && pack.playable, modifier = Modifier.weight(1f).heightIn(min = 48.dp)) {
                    Icon(Icons.Outlined.PlayArrow, contentDescription = null)
                    Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                    Text(stringResource(R.string.packs_start))
                }
                if (!pack.builtin) {
                    OutlinedButton(
                        onClick = onDelete, enabled = enabled, modifier = Modifier.heightIn(min = 48.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
                    ) {
                        Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.packs_delete) + "：" + pack.name)
                    }
                }
            }
        }
    }
}

@Composable
private fun MoreCard(onImport: () -> Unit, enabled: Boolean) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.TipsAndUpdates, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.size(8.dp))
                Text(stringResource(R.string.packs_more_title), style = MaterialTheme.typography.titleSmall)
            }
            Text(stringResource(R.string.packs_more_text), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            OutlinedButton(onClick = onImport, enabled = enabled, modifier = Modifier.heightIn(min = 48.dp)) {
                Icon(Icons.Outlined.FileOpen, contentDescription = null)
                Spacer(Modifier.size(ButtonDefaults.IconSpacing))
                Text(stringResource(R.string.packs_import_desc))
            }
        }
    }
}
