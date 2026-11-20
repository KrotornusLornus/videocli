-- lib/cli.lua — puente con el CLI videocli.
-- Requiere que los globales VC.util y VC.state estén cargados.

local util, state, ui = _G.VC.util, _G.VC.state, _G.VC.ui

local M = {}

-- El binario videocli debe estar en el PATH (symlink en ~/.local/bin).
local VIDEOCLI = "videocli"

function M.cmd(args)
  return VIDEOCLI .. " " .. args
end

-- Lista de miniaturas según los filtros activos.
function M.list(category, dir)
  local cmd = M.cmd("list")
  if category then
    cmd = cmd .. " --category " .. util.quote(category)
  end
  if dir then
    cmd = cmd .. " --dir " .. util.quote(dir)
  end
  return util.lines(cmd)
end

-- Recarga la galería desde la base de datos.
function M.refresh()
  local paths = M.list(state.filter_category, state.filter_dir)
  swayimg.imagelist.clear()
  if #paths > 0 then
    swayimg.imagelist.add(paths)
  end
  swayimg.gallery.reload()
  return #paths
end

-- Ruta del vídeo asociado a la miniatura seleccionada (o nil).
function M.selected_path()
  local entry = swayimg.gallery.get_image()
  if not entry then return nil end
  return entry.path
end

-- Reproduce los vídeos de las miniaturas indicadas.
-- Con music=true solo audio; con separate=true un proceso por vídeo.
function M.play_multi(paths, music, separate)
  if not paths or #paths == 0 then
    ui.status("Nada seleccionado")
    return
  end
  local cmd = "play"
  if music then cmd = cmd .. " --music" end
  if separate then cmd = cmd .. " --separate" end
  for _, p in ipairs(paths) do
    cmd = cmd .. " " .. util.quote(p)
  end
  local ok = os.execute(M.cmd(cmd) .. " >/dev/null 2>&1")
  local n = #paths
  if not ok then
    ui.status("Error al reproducir")
  elseif music then
    ui.status(("Audio: %d en cola"):format(n))
  elseif separate then
    ui.status(("Lanzados %d reproductores independientes"):format(n))
  else
    ui.status(("Reproduciendo %d en cola"):format(n))
  end
end

-- Rutas de los vídeos marcados (o de la selección actual si no hay marcas).
function M.marked_paths()
  local paths = {}
  for _, entry in ipairs(swayimg.imagelist.get()) do
    if entry.mark then
      table.insert(paths, entry.path)
    end
  end
  if #paths == 0 then
    local cur = M.selected_path()
    if cur then table.insert(paths, cur) end
  end
  return paths
end

-- Escanea una carpeta elegida con fuzzel (el CLI abre el selector).
-- Síncrono: bloquea hasta terminar, y después recarga la galería.
function M.scan()
  ui.status("Escaneando…")
  local ok, kind = os.execute(M.cmd("scan") .. " >/dev/null 2>&1")
  local n = M.refresh()
  if ok then
    ui.status(("Escaneado: %d vídeos en la galería"):format(n))
  else
    ui.status("Escaneo cancelado o con errores")
  end
end

-- Prompt de añadir/sacar categorías: casillas [x]/[ ] sobre los vídeos
-- marcados (o el actual). Comunes a todos → [x]; elegir saca, elegir una
-- [ ] añade. Bucle hasta Escape.
function M.category_toggle()
  local paths = M.marked_paths()
  if #paths == 0 then
    ui.status("Nada seleccionado")
    return
  end
  local cmd = "category toggle"
  for _, p in ipairs(paths) do
    cmd = cmd .. " " .. util.quote(p)
  end
  os.execute(M.cmd(cmd) .. " >/dev/null 2>&1")
  local n = M.refresh()
  ui.status(("Categorías actualizadas (%d vídeos visibles)"):format(n))
end

-- Copia al portapapeles la URL del vídeo (entrada importada).
function M.copy_url(path)
  if not path then ui.status("Nada seleccionado") return end
  local out = util.lines(M.cmd("url " .. util.quote(path)))
  if #out == 0 then
    ui.status("No hay URL (el vídeo no es una importación)")
    return
  end
  if os.execute(("printf %%s %s | wl-copy"):format(util.quote(out[1]))) then
    ui.status("URL copiada al portapapeles")
  else
    ui.status("No se pudo copiar (¿wl-copy disponible?)")
  end
end

-- Copia al portapapeles la ruta local del vídeo.
function M.copy_path(path)
  if not path then ui.status("Nada seleccionado") return end
  local out = util.lines(M.cmd("locate " .. util.quote(path)))
  if #out == 0 then
    ui.status("No hay fichero local para este vídeo")
    return
  end
  if os.execute(("printf %%s %s | wl-copy"):format(util.quote(out[1]))) then
    ui.status("Ruta local copiada al portapapeles")
  else
    ui.status("No se pudo copiar (¿wl-copy disponible?)")
  end
end

-- Elige categoría con fuzzel y refresca el filtro.
-- "todos" limpia ambos filtros y carga toda la videoteca;
-- "sin-categoria" son los vídeos sin ninguna categoría.
function M.choose_filter()
  local cats = { "todos", "sin-categoria" }
  for _, c in ipairs(util.lines(M.cmd("categories"))) do
    table.insert(cats, c)
  end
  local choice = util.fuzzel("Categoría> ", cats)
  if not choice then return end
  -- Cada filtro reemplaza al anterior (no se intersectan).
  state.filter_dir = nil
  if choice == "todos" then
    state.filter_category = nil
  else
    state.filter_category = choice
  end
  local n = M.refresh()
  if state.filter_category then
    local where = state.filter_dir or "toda la videoteca"
    ui.status(("Categoría: %s en %s (%d vídeos)"):format(state.filter_category, where, n))
  else
    ui.status(("Mostrando todos los vídeos (%d)"):format(n))
  end
end

-- Importar URLs: abre vi con una URL por línea y registra solo metadatos.
-- El worker de cada URL corre en segundo plano con yt-dlp.
function M.import_urls()
  ui.status("Importando URLs…")
  local ok = os.execute(M.cmd("import") .. " >/dev/null 2>&1")
  ui.status(ok and "URLs en cola de importación" or "Importación cancelada o con errores")
end

-- Descarga el vídeo de la entrada importada seleccionada.
function M.fetch_selected()
  local path = M.selected_path()
  if not path then ui.status("Nada seleccionado") return end
  local ok = os.execute(M.cmd("fetch " .. util.quote(path)) .. " >/dev/null 2>&1")
  ui.status(ok and "Descarga en curso (mira el estado más tarde)" or "No se pudo iniciar la descarga")
end

-- Eliminar: prompt con 4 niveles de borrado sobre la selección
-- (marcados con Space, o el actual si no hay marcas).
function M.remove_selected()
  local paths = M.marked_paths()
  if #paths == 0 then
    ui.status("Nada seleccionado")
    return
  end
  local prompt = ("Eliminar (%d vídeos) > "):format(#paths)
  local choice = util.fuzzel(prompt, {
    "de la vista actual",
    "los metadatos de la BD",
    "el vídeo local (no metadatos)",
    "todo (vídeo + metadatos)",
  }, true)
  if not choice then return end

  local ok
  if choice == "de la vista actual" then
    swayimg.imagelist.remove(paths)
    swayimg.gallery.reload()
    ui.status(("%d quitados de la vista (no se borra nada)"):format(#paths))
    return
  elseif choice == "los metadatos de la BD" then
    local cmd = "remove"
    for _, p in ipairs(paths) do cmd = cmd .. " " .. util.quote(p) end
    ok = os.execute(M.cmd(cmd) .. " >/dev/null 2>&1")
  elseif choice == "el vídeo local (no metadatos)" then
    local cmd = "delete-file"
    for _, p in ipairs(paths) do cmd = cmd .. " " .. util.quote(p) end
    ok = os.execute(M.cmd(cmd) .. " >/dev/null 2>&1")
  elseif choice == "todo (vídeo + metadatos)" then
    local delf, rem = "delete-file", "remove"
    for _, p in ipairs(paths) do
      delf = delf .. " " .. util.quote(p)
      rem = rem .. " " .. util.quote(p)
    end
    os.execute(M.cmd(delf) .. " >/dev/null 2>&1")
    ok = os.execute(M.cmd(rem) .. " >/dev/null 2>&1")
  end
  local n = M.refresh()
  if ok then
    ui.status(("%s: hecho (%d vídeos visibles)"):format(choice, n))
  else
    ui.status("No se pudo completar el borrado")
  end
end

-- Elige directorio con fuzzel: empieza en la carpeta actual de la galería
-- (~/Videos al arrancar) y se elige directamente la carpeta deseada.
function M.choose_dir()
  local start = state.filter_dir or util.expand(VC and _G.VC.start_dir or "~/Videos")
  local out = util.lines(M.cmd("pick-dir " .. util.quote(start)))
  if #out == 0 then
    ui.status("Navegación cancelada")
    return
  end
  -- Cada filtro reemplaza al anterior (no se intersectan).
  state.filter_category = nil
  state.filter_dir = out[1]
  local n = M.refresh()
  ui.status(("Directorio: %s (%d vídeos)"):format(state.filter_dir, n))
end

-- Regenera la miniatura: método local (ffmpeg) u online (yt-dlp).
function M.regen(path)
  if not path then ui.status("Nada seleccionado") return end
  local method = util.fuzzel("Regenerar miniatura> ", { "local", "online" })
  if not method then return end
  ui.status("Regenerando miniatura…")
  local ok = os.execute(
    M.cmd("thumb " .. util.quote(path) .. " --regenerate " .. util.quote(method))
    .. " >/dev/null 2>&1")
  swayimg.gallery.reload()
  ui.status(ok and ("Miniatura regenerada (%s)"):format(method)
             or "Error al regenerar la miniatura")
end

-- Resumen del vídeo seleccionado para la capa de estado.
function M.info(path)
  if not path then ui.status("Nada seleccionado") return end
  local out = util.lines(M.cmd("info " .. util.quote(path)))
  ui.status(#out > 0 and table.concat(out, "\n") or "Sin información")
end

-- Gestión global de categorías (crear/borrar) con fuzzel.
function M.category_manage()
  os.execute(M.cmd("category manage") .. " >/dev/null 2>&1")
  local n = M.refresh()
  ui.status(("Categorías actualizadas (%d vídeos visibles)"):format(n))
end

return M
