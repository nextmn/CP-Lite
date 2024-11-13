prefix = /usr/local
exec_prefix = $(prefix)
bindir = $(exec_prefix)/bin
BASHCOMPLETIONSDIR = $(exec_prefix)/share/bash-completion/completions


RM = rm -f
INSTALL = install -D

.PHONY: install uninstall build clean default
default: build
build:
	go build
clean:
	go clean
reinstall: uninstall install
install:
	$(INSTALL) cp-lite $(DESTDIR)$(bindir)/cp-lite
	$(INSTALL) bash-completion/completions/cp-lite $(DESTDIR)$(BASHCOMPLETIONSDIR)/cp-lite
	@echo "================================="
	@echo ">> Now run the following command:"
	@echo -e "\tsource $(DESTDIR)$(BASHCOMPLETIONSDIR)/cp-lite"
	@echo "================================="
uninstall:
	$(RM) $(DESTDIR)$(bindir)/cp-lite
	$(RM) $(DESTDIR)$(BASHCOMPLETIONSDIR)/cp-lite
