<config.mk

all:V: partage

%: %.go
	$GO build -o $stem

clean:V:
	rm -f partage

install:V: partage
	mkdir -p ${DESTDIR}${PREFIX}/bin
	cp partage ${DESTDIR}${PREFIX}/bin/partage
	chmod 755 ${DESTDIR}${PREFIX}/bin/partage
	mkdir -p ${DESTDIR}${MANDIR}/man1
	cp partage.1 ${DESTDIR}${MANDIR}/man1/partage.1
	chmod 644 ${DESTDIR}${MANDIR}/man1/partage.1

uninstall:V:
	rm ${DESTDIR}${PREFIX}/bin/partage
	rm ${DESTDIR}${MANDIR}/man1/partage.1
