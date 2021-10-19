<config.mk

all:V: partage partage-trash/partage-trash

%: %.go
	$GO build -o $stem $stem.go

clean:V:
	rm -f partage partage-trash/partage-trash

install:V: partage partage-trash/partage-trash
	mkdir -p ${DESTDIR}${PREFIX}/bin
	cp partage ${DESTDIR}${PREFIX}/bin/partage
	cp partage-trash/partage-trash ${DESTDIR}${PREFIX}/bin/partage-trash
	chmod 755 ${DESTDIR}${PREFIX}/bin/partage
	chmod 755 ${DESTDIR}${PREFIX}/bin/partage-trash
	mkdir -p ${DESTDIR}${MANDIR}/man1
	cp man/partage.1 ${DESTDIR}${MANDIR}/man1/partage.1
	cp man/partage-trash.1 ${DESTDIR}${MANDIR}/man1/partage-trash.1
	cp man/partage.conf.5 ${DESTDIR}${MANDIR}/man5/partage.conf.5
	chmod 644 ${DESTDIR}${MANDIR}/man1/partage.1
	chmod 644 ${DESTDIR}${MANDIR}/man1/partage-trash.1
	chmod 644 ${DESTDIR}${MANDIR}/man5/partage.conf.5

uninstall:V:
	rm ${DESTDIR}${PREFIX}/bin/partage
	rm ${DESTDIR}${PREFIX}/bin/partage-trash
	rm ${DESTDIR}${MANDIR}/man1/partage.1
	rm ${DESTDIR}${MANDIR}/man1/partage-trash.1
	rm ${DESTDIR}${MANDIR}/man5/partage.conf.5
