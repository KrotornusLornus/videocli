-- lib/keys.lua — atajos del modo galería (y de visor, lo mínimo).

local ui, cli = _G.VC.ui, _G.VC.cli

-- Galería ---------------------------------------------------------------

-- Movimiento tipo vim: h/j/k/l sobre la galería.
swayimg.gallery.on_key("h", function()
  swayimg.gallery.select("left")
end)
swayimg.gallery.on_key("j", function()
  swayimg.gallery.select("down")
end)
swayimg.gallery.on_key("k", function()
  swayimg.gallery.select("up")
end)
swayimg.gallery.on_key("l", function()
  swayimg.gallery.select("right")
end)

-- Reproducir la selección en cola (o el actual si no hay marcas).
swayimg.gallery.on_key("Return", function()
  cli.play_multi(cli.marked_paths(), false, false)
end)

-- Lanzar cada vídeo de la selección como proceso independiente.
swayimg.gallery.on_key("Shift+Return", function()
  cli.play_multi(cli.marked_paths(), false, true)
end)

-- Solo audio: siempre en una sola cola.
swayimg.gallery.on_key("m", function()
  cli.play_multi(cli.marked_paths(), true, false)
end)

-- Marcar/desmarcar la miniatura actual (selección múltiple).
swayimg.gallery.on_key("Space", function()
  swayimg.gallery.mark_image()
end)

-- Escanear carpeta con el selector fuzzel del CLI.
swayimg.gallery.on_key("s", function()
  cli.scan()
end)

-- Buscar/filtrar por categoría.
-- En teclado español "/" es Shift+7 → keysym "Shift+slash".
swayimg.gallery.on_key("Shift+slash", function()
  cli.choose_filter()
end)
swayimg.gallery.on_key("/", function()
  cli.choose_filter()
end)

-- Navegar/filtrar por directorios.
swayimg.gallery.on_key("Ctrl+f", function()
  cli.choose_dir()
end)

-- Añadir/sacar el vídeo de categorías (casillas; comunes a la selección).
swayimg.gallery.on_key("a", function()
  cli.category_toggle()
end)

-- Regenerar la miniatura del vídeo seleccionado.
swayimg.gallery.on_key("r", function()
  cli.regen(cli.selected_path())
end)

-- Información del vídeo seleccionado.
swayimg.gallery.on_key("i", function()
  cli.info(cli.selected_path())
end)

-- Gestión global de categorías (crear/renombrar/borrar).
swayimg.gallery.on_key("Shift+c", function()
  cli.category_manage()
end)

-- Pantalla completa.
swayimg.gallery.on_key("f", function()
  swayimg.fullscreen = not swayimg.fullscreen
end)

-- Eliminar selección: vista/BD/local/todo (prompt con fuzzel).
swayimg.gallery.on_key("Shift+d", function()
  cli.remove_selected()
end)

-- Tamaño de miniaturas: +/- (con límites razonables).
-- El + es Shift+= → keysym "plus" (y también aceptamos "=" a secas).
swayimg.gallery.on_key("plus", function()
  local n = swayimg.gallery.thumb_size + 20
  if n <= 512 then swayimg.gallery.thumb_size = n end
end)
swayimg.gallery.on_key("equal", function()
  local n = swayimg.gallery.thumb_size + 20
  if n <= 512 then swayimg.gallery.thumb_size = n end
end)
swayimg.gallery.on_key("minus", function()
  local n = swayimg.gallery.thumb_size - 20
  if n >= 64 then swayimg.gallery.thumb_size = n end
end)

-- Ayuda: ? / Shift+? (según teclado).
swayimg.gallery.on_key("question", function()
  ui.help()
end)
swayimg.gallery.on_key("Shift+question", function()
  ui.help()
end)

-- Salir.
swayimg.gallery.on_key("Escape", function()
  swayimg.exit()
end)

-- Importar URLs con solo metadatos (abre vi con una URL por línea).
swayimg.gallery.on_key("Shift+i", function()
  cli.import_urls()
end)

-- Copiar la URL de origen al portapapeles.
swayimg.gallery.on_key("y", function()
  cli.copy_url(cli.selected_path())
end)

-- Copiar la ruta local del vídeo al portapapeles.
swayimg.gallery.on_key("Shift+y", function()
  cli.copy_path(cli.selected_path())
end)

-- Descargar el vídeo de una entrada importada (solo metadatos).
swayimg.gallery.on_key("Ctrl+d", function()
  cli.fetch_selected()
end)