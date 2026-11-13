"""Агент отправляет запросы; чанкинг, эмбеддинги и SQL полностью выполняет Go."""

class HistoryMemory:
    def __init__(self, store):
        self.store = store

    def search(self, query):
        return self.store.search(query)

    def index(self, turn):
        # Сообщения уже сохранены: Go читает оригиналы самостоятельно.
        return self.store.index_run(turn)
